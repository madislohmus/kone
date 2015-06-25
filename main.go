package main

import (
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
	machines.Load1 = Measurement{Value: float32(l1)}

	l5, err := strconv.ParseFloat(loads[1], 32)
	if err != nil {
		l5 = -1
	}
	machines.Load5 = Measurement{Value: float32(l5)}

	l15, err := strconv.ParseFloat(loads[2], 32)
	if err != nil {
		l15 = -1
	}
	machines.Load15 = Measurement{Value: float32(l15)}

	free, err := strconv.ParseFloat(s[1], 32)
	if err != nil {
		free = -1
	}
	machines.Free = Measurement{Value: float32(free)}

	conn, err := strconv.ParseInt(s[2], 10, 32)
	if err != nil {
		conn = -1
	}
	machines.Connections = Measurement{Value: int32(conn)}

	nproc, err := strconv.ParseInt(s[3], 10, 32)
	if err != nil {
		nproc = -1
	}
	machines.Nproc = int32(nproc)

	stor, err := strconv.ParseInt(strings.TrimRight(s[4], "%"), 10, 32)
	if err != nil {
		stor = -1
	}
	machines.Storage = Measurement{Value: int32(stor)}

	inode, err := strconv.ParseInt(strings.TrimRight(s[5], "%"), 10, 32)
	if err != nil {
		inode = -1
	}
	machines.Inode = Measurement{Value: int32(inode)}

	ut, err := strconv.ParseFloat(s[6], 10)
	if err != nil {
		ut = -1
	}
	machines.Uptime = int64(ut)

	cpu, err := strconv.ParseFloat(s[7], 10)
	if err != nil {
		cpu = -1
	}
	machines.CPU = Measurement{Value: float32(cpu)}

}

func setMachineStatus(machines *Machine) {

	machines.Status = StatusOK

	machines.Status |= loadStatus(machines.Load1.Value.(float32), machines.Nproc)
	machines.Status |= loadStatus(machines.Load5.Value.(float32), machines.Nproc)
	machines.Status |= loadStatus(machines.Load15.Value.(float32), machines.Nproc)

	cpu := machines.CPU.Value.(float32)
	if cpu >= 90*(float32(machines.Nproc)) {
		machines.Status |= StatusError
	} else if cpu >= 80*(float32(machines.Nproc)) {
		machines.Status |= StatusWarning
	}

	free := machines.Free.Value.(float32)
	if free >= 0.9 {
		machines.Status |= StatusError
	} else if free >= 0.8 {
		machines.Status |= StatusWarning
	}

	storage := machines.Storage.Value.(int32)
	if storage >= 90 {
		machines.Status |= StatusError
	} else if storage >= 80 {
		machines.Status |= StatusWarning
	}

	inode := machines.Inode.Value.(int32)
	if inode >= 90 {
		machines.Status |= StatusError
	} else if inode >= 80 {
		machines.Status |= StatusWarning
	}

	conns := machines.Connections.Value.(int32)
	if conns > 50000 {
		machines.Status |= StatusError
	} else if conns > 10000 {
		machines.Status |= StatusWarning
	}

}

func loadStatus(load float32, nproc int32) int {
	if load < float32(nproc)*0.8 {
		return StatusOK
	} else if load < float32(nproc) {
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
	for _, line := range strings.Split(string(data), "\n") {
		m := strings.Split(line, ",")
		if len(m) > 3 {
			config := gosh.Config{
				User:    strings.TrimSpace(m[1]),
				Host:    strings.TrimSpace(m[2]),
				Port:    strings.TrimSpace(m[3]),
				Timeout: 15 * time.Second,
				Signer:  signer}
			machines[strings.TrimSpace(m[0])] = &Machine{
				Name:   strings.TrimSpace(m[0]),
				config: &config}
		}
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
