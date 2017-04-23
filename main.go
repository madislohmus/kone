package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/madislohmus/gosh"
	"golang.org/x/crypto/ssh"
)

type (
	consulCheck struct {
		Status string `json:"Status"`
	}
)

const (
	loadCmd        = `cat /proc/loadavg | awk '{print $1,$2,$3}'`
	freeCmd        = `if [ "$(free | grep available)" ]; then free | grep Mem | awk '{print ($2-$7)/$2}'; else free | grep Mem | awk '{print ($3-$6-$7)/$2}'; fi`
	connsCmd       = `netstat -ant | awk '{print $5}' | uniq -u | wc -l`
	procCmd        = `nproc`
	storageCmd     = `df -x tmpfs -x none | grep '/' | grep '%' | awk '{print $6 "=" $5}' | sort -g | awk '{printf "%s ",$0} END {print " "}'`
	inodeCmd       = `df -i -x tmpfs -x none | grep '/' | grep '%' | awk '{print $6 "=" $5}' | sort -g | awk '{printf "%s ",$0} END {print " "}'`
	uptimeCmd      = `cat /proc/uptime | awk '{print $1}'`
	cpuUtilCmd     = `top -b -n2 | grep "Cpu(s)"| tail -n 1 | awk '{print $2 + $4}'`
	consulServices = `curl -s http://localhost:8500/v1/health/node/$(hostname)`
	command        = loadCmd + ` &&  ` + freeCmd + `&& ` + connsCmd + ` && ` + procCmd + ` && ` + storageCmd + ` && ` + inodeCmd + ` && ` + uptimeCmd + ` && ` + cpuUtilCmd + `&&` + consulServices
)

var (
	wg                   sync.WaitGroup
	machines             map[string]*machine
	signer               *ssh.Signer
	sorter               machineSorter
	running              bool
	fetchTime            time.Time
	sortRequestChannel   chan bool
	redrawRequestChannel chan bool
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
			Signer:  signer}
		m.config = &config
		machines[m.Name] = m
	}
	return nil
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

func updateRoutine() {
	for {
		if !running {
			fetchTime = time.Now()
			drawDate()
			runOnHosts(false)
		}
		time.Sleep(time.Duration(*sleepTime) * time.Second)
	}
}

func main() {
	signer = getSigner()
	if signer == nil {
		return
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
	go updateRoutine()
	runCli()
}
