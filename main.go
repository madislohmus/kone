package main

import (
	"fmt"
	"github.com/madislohmus/gosh"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	wg        sync.WaitGroup
	command   = `uptime | awk '{print $(NF-2) $(NF-1) $NF}' && free | grep Mem | awk '{print ($3-$6-$7)/$2}' && netstat -ant | wc -l && nproc && df -h / | grep '/' | awk '{print $5}' && cat /proc/uptime | awk '{print $1}' && top -b -n2 | grep "Cpu(s)"|tail -n 1 | awk '{print $2 + $4}'`
	machines  map[string]*Machine
	signer    *ssh.Signer
	sorter    Sorter
	data      map[string]*Data
	running   bool
	fetchTime time.Time
)

func RunOnHost(machine string, forceReConnect bool) {
	if data[machine].Fetching {
		return
	}
	wg.Add(1)
	runOnHost(command, machine, forceReConnect)
	wg.Wait()
}

func RunOnHosts(forceReConnect bool) {
	for k, _ := range machines {
		if !data[k].Fetching {
			wg.Add(1)
			runOnHost(command, k, forceReConnect)
		}
	}
	wg.Wait()
}

func runOnHost(command string, machine string, forceReConnect bool) {
	go func(key string) {
		data[key].Fetching = true
		var err error
		var result string
		if machines[key].client == nil || forceReConnect {
			if forceReConnect {
				fmt.Printf("FORCING RECONNECT")
			}
			machines[key].client, err = gosh.GetClient(*machines[key].config, 15*time.Second)
		}
		if machines[key].client != nil {
			result, err = gosh.RunOnClient(command, *machines[key].client, 15*time.Second)
		}
		data[key].Fetching = false
		if err != nil {
			data[key].GotResult = false
			data[key].FetchingError = err.Error()
			data[key].Status |= StatusUnknown
		} else {
			data[key].GotResult = true
			populate(data[key], result)
			setMachineStatus(data[key])
		}
		sort.Sort(sorter)
		formatMachine(key)
		redraw()
		wg.Done()
	}(machine)
}

func populate(data *Data, result string) {
	s := strings.Split(result, "\n")
	loads := strings.Split(s[0], ",")
	l1, err := strconv.ParseFloat(loads[0], 32)
	if err != nil {
		data.Load1 = -1
	} else {
		data.Load1 = float32(l1)
	}

	l5, err := strconv.ParseFloat(loads[1], 32)
	if err != nil {
		data.Load5 = -1
	} else {
		data.Load5 = float32(l5)
	}

	l15, err := strconv.ParseFloat(loads[2], 32)
	if err != nil {
		data.Load15 = -1
	} else {
		data.Load15 = float32(l15)
	}

	free, err := strconv.ParseFloat(s[1], 32)
	if err != nil {
		free = -1
	} else {
		data.Free = float32(free)
	}

	conn, err := strconv.ParseInt(s[2], 10, 32)
	if err != nil {
		conn = -1
	} else {
		data.Connections = int32(conn)
	}

	nproc, err := strconv.ParseInt(s[3], 10, 32)
	if err != nil {
		nproc = -1
	} else {
		data.Nproc = int32(nproc)
	}

	stor, err := strconv.ParseInt(strings.TrimRight(s[4], "%"), 10, 32)
	if err != nil {
		stor = -1
	} else {
		data.Storage = int32(stor)
	}

	ut, err := strconv.ParseFloat(s[5], 10)
	if err != nil {
		ut = -1
	} else {
		data.Uptime = int64(ut)
	}

	cpu, err := strconv.ParseFloat(s[6], 10)
	if err != nil {
		cpu = -1
	} else {
		data.CPU = float32(cpu)
	}

}

func setMachineStatus(data *Data) {

	data.Status = StatusOK

	data.Status |= loadStatus(data.Load1, data.Nproc)
	data.Status |= loadStatus(data.Load5, data.Nproc)
	data.Status |= loadStatus(data.Load15, data.Nproc)

	if data.CPU >= 80*(float32(data.Nproc)) {
		data.Status |= StatusWarning
	} else if data.CPU >= 90*(float32(data.Nproc)) {
		data.Status |= StatusError
	}

	if data.Free >= 0.8 {
		data.Status |= StatusWarning
	} else if data.Free >= 0.9 {
		data.Status |= StatusError
	}

	if data.Storage >= 80 {
		data.Status |= StatusWarning
	} else if data.Storage >= 90 {
		data.Status |= StatusError
	}

	if data.Connections > 10000 {
		data.Status |= StatusWarning
	} else if data.Connections > 50000 {
		data.Status |= StatusError
	}

}

func loadStatus(load float32, nproc int32) int {
	if load >= 0.9*(float32(nproc)) {
		return StatusError
	} else if load >= 0.8*(float32(nproc)) {
		return StatusError
	}
	return StatusOK
}

func getPassword() ([]byte, error) {
	data, err := ioutil.ReadFile(*passFile)
	if err != nil {
		return nil, err
	}
	if data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	return data, nil
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
	data = make(map[string]*Data)
	for k, v := range machines {
		sorter.keys = append(sorter.keys, k)
		data[k] = &Data{Machine: k, IP: v.config.Host}
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
