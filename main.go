package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/madislohmus/gosh"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type (
	consulCheck struct {
		Status string `json:"Status"`
	}
)

const (
	hashType         = "1"
	unixNetwork      = "unix"
	updateTimeMillis = 60000 * 5
	loadCmd          = `cat /proc/loadavg | awk '{print $1,$2,$3}'`
	freeCmd          = `if [ "$(free | grep available)" ]; then free | grep Mem | awk '{print ($2-$7)/$2}'; else free | grep Mem | awk '{print ($3-$6-$7)/$2}'; fi`
	connsCmd         = `netstat -ant | awk '{print $5}' | uniq -u | wc -l`
	procCmd          = `nproc`
	storageCmd       = `df -x tmpfs -x none | grep '/' | grep '%' | awk '{print $6 "=" $5}' | sort -g | awk '{printf "%s ",$0} END {print " "}'`
	inodeCmd         = `df -i -x tmpfs -x none | grep '/' | grep '%' | awk '{print $6 "=" $5}' | sort -g | awk '{printf "%s ",$0} END {print " "}'`
	uptimeCmd        = `cat /proc/uptime | awk '{print $1}'`
	cpuUtilCmd       = `top -b -n2 | grep "Cpu(s)"| tail -n 1 | awk '{print $2 + $4}'`
	consulServices   = `curl -s http://localhost:8500/v1/health/node/$(hostname)`
	command          = loadCmd + ` &&  ` + freeCmd + `&& ` + connsCmd + ` && ` + procCmd + ` && ` + storageCmd + ` && ` + inodeCmd + ` && ` + uptimeCmd + ` && ` + cpuUtilCmd + `&&` + consulServices
)

var (
	wg                   sync.WaitGroup
	machines             map[string]*machine
	signers              []ssh.Signer
	sorter               machineSorter
	running              bool
	fetchTime            time.Time
	sortRequestChannel   chan bool
	redrawRequestChannel chan bool
	sshAuthSocket        = os.Getenv("SSH_AUTH_SOCK")
)

func runOnHost(machine string, forceReConnect bool) {
	if machines[machine].Fetching {
		return
	}
	wg.Add(1)
	go runCommandOnHost(command, machine, forceReConnect)
	wg.Wait()
}

func runOnHosts(forceReConnect bool) {
	for k := range machines {
		if !machines[k].Fetching {
			wg.Add(1)
			go runCommandOnHost(command, k, forceReConnect)
		}
	}
	wg.Wait()
}

func runCommandOnHost(command string, machine string, forceReConnect bool) {
	machines[machine].Fetching = true
	sendRedrawRequest()
	var err error
	var result string
	if machines[machine].client == nil || forceReConnect {
		machines[machine].client, err = gosh.GetClient(*machines[machine].config, 15*time.Second)
	}
	if machines[machine].client != nil {
		result, err = gosh.RunOnClient(command, *machines[machine].client, 15*time.Second)
		if op, ok := err.(*net.OpError); ok {
			if !op.Timeout() {
				machines[machine].client = nil
			}
		}
	}
	machines[machine].Fetching = false
	if err != nil {
		machines[machine].GotResult = false
		machines[machine].FetchingError = err.Error()
		machines[machine].Status |= statusUnknown
	} else {
		machines[machine].GotResult = true
		populate(machines[machine], result)
		setMachineStatus(machines[machine])
	}
	formatMachine(machine)
	sendSortingRequest()
	wg.Done()
}

func sendSortingRequest() {
	sortRequestChannel <- true
}

func sortingRoutine() {
	for {
		<-sortRequestChannel
		for len(sortRequestChannel) > 0 {
			<-sortRequestChannel
		}
		sort.Sort(sorter)
		sendRedrawRequest()
	}
}

func populate(machines *machine, result string) {
	s := strings.Split(result, "\n")
	loads := strings.Split(s[0], " ")
	l1, err := strconv.ParseFloat(loads[0], 32)
	if err != nil {
		l1 = -1
	}
	machines.Load1.Value = float32(l1)

	l5, err := strconv.ParseFloat(loads[1], 32)
	if err != nil {
		l5 = -1
	}
	machines.Load5.Value = float32(l5)

	l15, err := strconv.ParseFloat(loads[2], 32)
	if err != nil {
		l15 = -1
	}
	machines.Load15.Value = float32(l15)

	free, err := strconv.ParseFloat(s[1], 32)
	if err != nil {
		free = -1
	}
	machines.Free.Value = float32(free)

	conn, err := strconv.ParseInt(s[2], 10, 32)
	if err != nil {
		conn = -1
	}
	machines.Connections.Value = int32(conn)

	nproc, err := strconv.ParseInt(s[3], 10, 32)
	if err != nil {
		nproc = -1
	}
	machines.Nproc = int32(nproc)

	driveUsages := strings.Split(strings.TrimSpace(s[4]), " ")
	drives := []int32{}
	for _, usage := range driveUsages {
		stor, err := strconv.ParseInt(strings.TrimRight(strings.Split(usage, "=")[1], "%"), 10, 32)
		if err != nil {
			stor = -1
		}
		drives = append(drives, int32(stor))
	}
	machines.Storage.Value = drives

	inodeUsages := strings.Split(strings.TrimSpace(s[5]), " ")
	inodes := []int32{}
	for _, usage := range inodeUsages {
		stor, err := strconv.ParseInt(strings.TrimRight(strings.Split(usage, "=")[1], "%"), 10, 32)
		if err != nil {
			stor = -1
		}
		inodes = append(inodes, int32(stor))
	}
	machines.Inode.Value = inodes

	ut, err := strconv.ParseFloat(s[6], 10)
	if err != nil {
		ut = -1
	}
	machines.Uptime.Value = int64(ut)

	cpu, err := strconv.ParseFloat(s[7], 10)
	if err != nil {
		cpu = -1
	}
	machines.CPU.Value = float32(cpu)

	consulChecks := strings.TrimSpace(s[8])
	if len(consulChecks) > 0 {
		var checks []consulCheck
		err := json.Unmarshal([]byte(consulChecks), &checks)
		if err == nil {
			checkArray := [4]int32{}
			for _, check := range checks {
				switch check.Status {
				case "passing":
					checkArray[0]++
				case "unknown":
					checkArray[1]++
				case "warning":
					checkArray[2]++
				case "critical":
					fallthrough
				default:
					checkArray[3]++
				}
			}
			machines.Services.Value = checkArray
		}
	}
}

func setMachineStatus(machine *machine) {

	machine.Status = statusOK

	machine.Status |= getLoadStatus(machine, machine.Load1)
	machine.Status |= getLoadStatus(machine, machine.Load5)
	machine.Status |= getLoadStatus(machine, machine.Load15)
	machine.Status |= getCPUStatus(machine)
	machine.Status |= getFreeStatus(machine)
	machine.Status |= getStorageStatus(machine)
	machine.Status |= getInodeStatus(machine)
	machine.Status |= getConnectionsStatus(machine)
	machine.Status |= getUptimeStatus(machine)
	machine.Status |= getServicesStatus(machine)

}

func getCPUStatus(machine *machine) int {
	cpu := machine.CPU.Value.(float32)
	warn, ok := machine.CPU.Warning.(float64)
	if !ok {
		warn = 80
	}
	err, ok := machine.CPU.Error.(float64)
	if !ok {
		err = 90
	}
	if cpu < float32(warn)*(float32(machine.Nproc)) {
		return statusOK
	} else if cpu < float32(err)*(float32(machine.Nproc)) {
		return statusWarning
	}
	return statusError
}

func getFreeStatus(machine *machine) int {
	free := machine.Free.Value.(float32)
	warn, ok := machine.Free.Warning.(float64)
	if !ok {
		warn = 0.8
	}
	err, ok := machine.Free.Error.(float64)
	if !ok {
		err = 0.9
	}
	if free < float32(warn) {
		return statusOK
	} else if free < float32(err) {
		return statusWarning
	}
	return statusError
}

func getStorageStatus(machine *machine) int {
	warn, ok := machine.Storage.Warning.(float64)
	if !ok {
		warn = 80
	}
	err, ok := machine.Storage.Error.(float64)
	if !ok {
		err = 90
	}
	status := statusOK
	for _, value := range machine.Storage.Value.([]int32) {
		status |= getSingleStorageStatus(value, warn, err)
	}
	return status
}

func getSingleStorageStatus(value int32, warn, err float64) int {
	if value < int32(warn) {
		return statusOK
	} else if value < int32(err) {
		return statusWarning
	}
	return statusError
}

func getInodeStatus(machine *machine) int {
	warn, ok := machine.Inode.Warning.(float64)
	if !ok {
		warn = 80
	}
	err, ok := machine.Inode.Error.(float64)
	if !ok {
		err = 90
	}
	status := statusOK
	for _, value := range machine.Inode.Value.([]int32) {
		status |= getSingleStorageStatus(value, warn, err)
	}
	return status
}

func getConnectionsStatus(machine *machine) int {
	conns := machine.Connections.Value.(int32)
	warn, ok := machine.Connections.Warning.(float64)
	if !ok {
		warn = 52429
	}
	err, ok := machine.Connections.Error.(float64)
	if !ok {
		err = 58982
	}
	if conns < int32(warn) {
		return statusOK
	} else if conns < int32(err) {
		return statusWarning
	}
	return statusError
}

func getLoadStatus(machine *machine, load measurement) int {
	l := load.Value.(float32)
	nproc := machine.Nproc
	warn, ok := load.Warning.(float64)
	if !ok {
		warn = 0.8 * float64(nproc)
	}
	err, ok := load.Error.(float64)
	if !ok {
		err = float64(nproc)
	}
	if l < float32(warn) {
		return statusOK
	} else if l < float32(err) {
		return statusWarning
	}
	return statusError
}

func getUptimeStatus(machine *machine) int {
	ut := machine.Uptime.Value.(int64)
	warn, ok := machine.Uptime.Warning.(float64)
	if !ok {
		warn = 90 * 24 * 60 * 60
	}
	err, ok := machine.Uptime.Error.(float64)
	if !ok {
		err = 100 * 24 * 60 * 60
	}
	if ut < int64(warn) {
		return statusOK
	} else if ut < int64(err) {
		return statusWarning
	}
	return statusError
}

func getServicesStatus(machine *machine) int {
	if machine.Services.Value != nil {
		checks := machine.Services.Value.([4]int32)
		if checks[3] > 0 {
			return statusError
		} else if checks[2] > 0 {
			return statusWarning
		} else if checks[1] > 0 {
			return statusUnknown
		}
	}
	return statusOK
}

func getPassword() ([]byte, error) {
	machines, err := ioutil.ReadFile(*passFile)
	if err != nil {
		return nil, err
	}
	if machines[len(machines)-1] == '\n' {
		machines = machines[:len(machines)-1]
	}
	return machines, nil
}

func populateMachines() error {
	data, err := ioutil.ReadFile(*dataFile)
	if err != nil {
		return err
	}
	machines = make(map[string]*machine)
	var ms []*machine
	err = json.Unmarshal(data, &ms)
	if err != nil {
		return err
	}
	for _, m := range ms {
		config := gosh.Config{
			User:    m.User,
			Host:    m.Host,
			Port:    m.Port,
			Timeout: 15 * time.Second,
			Signers: signers}
		m.config = &config
		machines[m.Name] = m
	}
	if len(*knownHosts) > 0 {
		err = populatePublicKeys(machines)
		if err != nil {
			return err
		}
	}
	return nil
}

func populatePublicKeys(machines map[string]*machine) error {
	data, err := ioutil.ReadFile(*knownHosts)
	if err != nil {
		return err
	}
	hostToKey := make(map[string]ssh.PublicKey)
	_, hosts, publicKey, _, rest, err := ssh.ParseKnownHosts(data)
	for err == nil {
		for _, host := range hosts {
			hostToKey[host] = publicKey
		machineLoop:
			for _, machine := range machines {
				if net.ParseIP(host) != nil {
					if host == machine.Host {
						machine.config.PublicKey = publicKey
						machines[machine.Name] = machine
						break machineLoop
					}
				} else {
					ipPat := regexp.MustCompile("\\[(.+?)\\]")
					ips := ipPat.FindStringSubmatch(host)
					if len(ips) > 1 {
						if ips[1] == machine.Host {
							machine.config.PublicKey = publicKey
							machines[ips[1]] = machine
							break machineLoop
						}
					} else {
						// host is hashed, lets hash machine and check if match found
						hashedHost, err := getHashForHost(host, machine.Host)
						if err != nil {
							return err
						}
						if hashedHost == host {
							machine.config.PublicKey = publicKey
							machines[machine.Name] = machine
							break machineLoop
						}
					}
				}
			}
		}
		_, hosts, publicKey, _, rest, err = ssh.ParseKnownHosts(rest)
	}
	if err != io.EOF {
		return err
	}
	return nil
}

func getHashForHost(host, ip string) (string, error) {
	splitter := "|"
	chunks := strings.Split(host, splitter)
	if len(chunks) != 4 {
		message := fmt.Sprintf("Expected 3 cunks, got %d: %s", len(chunks), host)
		return "", errors.New(message)
	}
	if chunks[1] == hashType {
		encodedSalt := chunks[2]
		salt, err := base64.StdEncoding.DecodeString(encodedSalt)
		if err != nil {
			return "", err
		}
		mac := hmac.New(sha1.New, salt)
		mac.Write([]byte(ip))
		hash := mac.Sum(nil)
		encodedHash := base64.StdEncoding.EncodeToString(hash)
		parts := []string{"", hashType, encodedSalt, encodedHash}
		return strings.Join(parts, splitter), nil
	} else {
		return "", fmt.Errorf("Expected hash type to be '%s'", hashType)
	}
}

func getSignersFromAgent() ([]ssh.Signer, error) {
	sock, err := net.Dial(unixNetwork, sshAuthSocket)
	if err != nil {
		return nil, err
	}

	return agent.NewClient(sock).Signers()
}

func getSigner() *ssh.Signer {
	var p []byte
	if *passFile != "" {
		pass, err := getPassword()
		if err != nil {
			fmt.Printf("Could not get password!")
			return nil
		}
		p = pass
	}

	s, err := gosh.GetSigner(*keyFile, string(p))
	if err != nil {
		fmt.Println("Could not get signer!")
		return nil
	}
	return s
}

func fetchRandomMachine() {
	fetchTime = time.Now()
	drawDate()
	machineNr := rand.Int31n(int32(len(machines)))
	key := sorter.keys[machineNr]
	if !machines[key].Fetching {
		wg.Add(1)
		go runCommandOnHost(command, key, false)
		wg.Wait()
	}
}

func updateRoutine() {
	for {
		go fetchRandomMachine()
		time.Sleep(time.Duration(updateTimeMillis/len(machines)) * time.Millisecond)
	}
}

func initialFetch() {
	if !running {
		fetchTime = time.Now()
		drawDate()
		runOnHosts(false)
	}
}

func main() {
	rand.Seed(time.Now().Unix())
	var err error
	signers, err = getSignersFromAgent()
	if len(signers) == 0 || err != nil {
		signer := getSigner()
		if signer == nil {
			return
		}
		signers = append(signers, *signer)
	}

	if err := populateMachines(); err != nil {
		fmt.Printf("%s", err.Error())
		return
	}
	initMachines(machines)
	sorter = machineSorter{keyToIndex: make(map[string]int)}
	for k := range machines {
		sorter.keys = append(sorter.keys, k)
		formatMachine(k)
	}
	sortRequestChannel = make(chan bool, 10)
	redrawRequestChannel = make(chan bool, 10)
	go redrawRoutine()
	go sortingRoutine()
	go initialFetch()
	go updateRoutine()
	runCli()
}
