package main

import (
	"encoding/json"
	"fmt"
	"github.com/madislohmus/gosh"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	wg        sync.WaitGroup
	command   = `uptime | awk '{print $(NF-2) $(NF-1) $NF}' && free | grep Mem | awk '{print ($3-$6-$7)/$2}' && netstat -ant | awk '{print $5}' | uniq -u | wc -l && nproc && df / | grep '/' | awk '{print $5}' && df -i / | grep '/' | awk '{print $5}' && cat /proc/uptime | awk '{print $1}' && top -b -n2 | grep "Cpu(s)"|tail -n 1 | awk '{print $2 + $4}'`
	machines  map[string]*Machine
	signer    *ssh.Signer
	sorter    Sorter
	running   bool
	fetchTime time.Time
)

func RunOnHost(machine string, forceReConnect bool) {
	if machines[machine].Fetching {
		return
	}
	wg.Add(1)
	runOnHost(command, machine, forceReConnect)
	wg.Wait()
}

func RunOnHosts(forceReConnect bool) {
	for k, _ := range machines {
		if !machines[k].Fetching {
			wg.Add(1)
			runOnHost(command, k, forceReConnect)
		}
	}
	wg.Wait()
}

func runOnHost(command string, machine string, forceReConnect bool) {
	go func(key string) {
		machines[key].Fetching = true
		var err error
		var result string
		if machines[key].client == nil || forceReConnect {
			machines[key].client, err = gosh.GetClient(*machines[key].config, 15*time.Second)
		}
		if machines[key].client != nil {
			result, err = gosh.RunOnClient(command, *machines[key].client, 15*time.Second)
			if op, ok := err.(*net.OpError); ok {
				if !op.Timeout() {
					machines[key].client = nil
				}
			}
		}
		machines[key].Fetching = false
		if err != nil {
			machines[key].GotResult = false
			machines[key].FetchingError = err.Error()
			machines[key].Status |= StatusUnknown
		} else {
			machines[key].GotResult = true
			populate(machines[key], result)
			setMachineStatus(machines[key])
		}
		sort.Sort(sorter)
		formatMachine(key)
		redraw()
		wg.Done()
	}(machine)
}

func populate(machines *Machine, result string) {
	s := strings.Split(result, "\n")
	loads := strings.Split(s[0], ",")
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

	stor, err := strconv.ParseInt(strings.TrimRight(s[4], "%"), 10, 32)
	if err != nil {
		stor = -1
	}
	machines.Storage.Value = int32(stor)

	inode, err := strconv.ParseInt(strings.TrimRight(s[5], "%"), 10, 32)
	if err != nil {
		inode = -1
	}
	machines.Inode.Value = int32(inode)

	ut, err := strconv.ParseFloat(s[6], 10)
	if err != nil {
		ut = -1
	}
	machines.Uptime = int64(ut)

	cpu, err := strconv.ParseFloat(s[7], 10)
	if err != nil {
		cpu = -1
	}
	machines.CPU.Value = float32(cpu)

}

func setMachineStatus(machine *Machine) {

	machine.Status = StatusOK

	machine.Status |= getLoadStatus(machine, machine.Load1.Value.(float32))
	machine.Status |= getLoadStatus(machine, machine.Load5.Value.(float32))
	machine.Status |= getLoadStatus(machine, machine.Load15.Value.(float32))
	machine.Status |= getCPUStatus(machine)
	machine.Status |= getFreeStatus(machine)
	machine.Status |= getStorageStatus(machine)
	machine.Status |= getInodeStatus(machine)
	machine.Status |= getConnectionsStatus(machine)

}

func getCPUStatus(machine *Machine) int {
	cpu := machine.CPU.Value.(float32)
	warn, ok := machine.CPU.Warning.(float64)
	if !ok {
		warn = 90
	}
	err, ok := machine.CPU.Warning.(float64)
	if !ok {
		err = 80
	}
	if cpu < float32(warn)*(float32(machine.Nproc)) {
		return StatusOK
	} else if cpu < float32(err)*(float32(machine.Nproc)) {
		return StatusWarning
	}
	return StatusError
}

func getFreeStatus(machine *Machine) int {
	free := machine.Free.Value.(float32)
	warn, ok := machine.Free.Warning.(float64)
	if !ok {
		warn = 0.8
	}
	err, ok := machine.Free.Warning.(float64)
	if !ok {
		err = 0.9
	}
	if free < float32(warn) {
		return StatusOK
	} else if free < float32(err) {
		return StatusWarning
	}
	return StatusError
}

func getStorageStatus(machine *Machine) int {
	storage := machine.Storage.Value.(int32)
	warn, ok := machine.Storage.Warning.(float64)
	if !ok {
		warn = 80
	}
	err, ok := machine.Storage.Warning.(float64)
	if !ok {
		err = 90
	}
	if storage < int32(warn) {
		return StatusOK
	} else if storage < int32(err) {
		return StatusWarning
	}
	return StatusError
}

func getInodeStatus(machine *Machine) int {
	inode := machine.Inode.Value.(int32)
	warn, ok := machine.Inode.Warning.(float64)
	if !ok {
		warn = 80
	}
	err, ok := machine.Inode.Warning.(float64)
	if !ok {
		err = 90
	}
	if inode < int32(warn) {
		return StatusOK
	} else if inode < int32(err) {
		return StatusWarning
	}
	return StatusError
}

func getConnectionsStatus(machine *Machine) int {
	conns := machine.Connections.Value.(int32)
	warn, ok := machine.Connections.Warning.(float64)
	if !ok {
		warn = 10000
	}
	err, ok := machine.Connections.Warning.(float64)
	if !ok {
		err = 50000
	}
	if conns < int32(warn) {
		return StatusOK
	} else if conns < int32(err) {
		return StatusWarning
	}
	return StatusError
}

func getLoadStatus(machine *Machine, load float32) int {
	nproc := machine.Nproc
	warn, ok := machine.Connections.Warning.(float64)
	if !ok {
		warn = 0.8 * float64(nproc)
	}
	err, ok := machine.Connections.Warning.(float64)
	if !ok {
		err = float64(nproc)
	}
	if load < float32(warn) {
		return StatusOK
	} else if load < float32(err) {
		return StatusWarning
	}
	return StatusError
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
	machines = make(map[string]*Machine)
	var ms []*Machine
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

func main() {
	var p []byte
	if *passFile != "" {
		pass, err := getPassword()
		if err != nil {
			fmt.Printf("Could not get password!")
			return
		}
		p = pass
	}
	s, err := gosh.GetSigner(*keyFile, string(p))
	if err != nil {
		fmt.Println("Could not get signer!")
		return
	}
	signer = s
	if err := populateMachines(); err != nil {
		fmt.Printf("%s", err.Error())
		return
	}
	Init(machines)
	for k, _ := range machines {
		sorter.keys = append(sorter.keys, k)
		formatMachine(k)
	}
	sort.Sort(sorter)

	go func() {
		for {
			if !running {
				fetchTime = time.Now()
				drawDate()
				RunOnHosts(false)
			}
			time.Sleep(time.Duration(*sleepTime) * time.Second)
		}
	}()
	runCli()

}
