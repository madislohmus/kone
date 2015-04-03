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
	wg         sync.WaitGroup
	command    = `uptime | awk '{print $(NF-2) $(NF-1) $NF}' && free | grep Mem | awk '{print ($3-$6-$7)/$2}' && netstat -ant | wc -l && nproc && df -h / | grep '/' | awk '{print $5}' && cat /proc/uptime | awk '{print $1}' && top -b -n2 | grep "Cpu(s)"|tail -n 1 | awk '{print $2 + $4}'`
	machines   map[string]Machine
	signer     *ssh.Signer
	sortedKeys []string
	data       map[string]*Data
	running    bool
	fetchTime  time.Time
)

func runAllHosts(command string) {
	running = true
	for k, _ := range machines {
		wg.Add(1)
		go func(key string) {
			data[key].Fetching = true
			drawMachine(key)
			result, err := gosh.Run(command, machines[key].User, machines[key].IP, machines[key].Port, signer)
			if err != nil {
				data[key].GotResult = false
				data[key].Fetching = false
				drawMachine(key)
			} else {
				data[key].GotResult = true
				data[key].Fetching = false
				populate(data[key], result)
				drawMachine(key)
			}
			wg.Done()
		}(k)
	}
	wg.Wait()
	running = false
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
		data.Load1 = -1
	} else {
		data.Load5 = float32(l5)
	}

	l15, err := strconv.ParseFloat(loads[2], 32)
	if err != nil {
		data.Load1 = -1
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
	machines = make(map[string]Machine)
	for _, line := range strings.Split(string(data), "\n") {
		m := strings.Split(line, ",")
		if len(m) > 3 {
			machines[strings.TrimSpace(m[0])] = Machine{
				Name: strings.TrimSpace(m[0]),
				User: strings.TrimSpace(m[1]),
				IP:   strings.TrimSpace(m[2]),
				Port: strings.TrimSpace(m[3])}
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
	if err := populateMachines(); err != nil {
		fmt.Printf("%s", err.Error())
		return
	}
	Init(machines)
	signer = s
	data = make(map[string]*Data)
	for k, _ := range machines {
		sortedKeys = append(sortedKeys, k)
		data[k] = &Data{Machine: k}
	}

	srt := Sorter{data: sortedKeys}
	sort.Sort(srt)

	go func() {
		for {
			if !running {
				fetchTime = time.Now()
				drawDate()
				runAllHosts(command)
			}
			time.Sleep(time.Duration(*sleepTime) * time.Second)
		}
	}()
	runCli()
}
