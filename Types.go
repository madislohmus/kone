package main

import (
	"flag"
	"io/ioutil"
	"os"
	"os/user"
	"strings"

	"github.com/madislohmus/gosh"
	"github.com/nsf/termbox-go"
	"golang.org/x/crypto/ssh"
)

type (
	textInColumns struct {
		ColumnWidth     map[string]int
		ColumnAlignment map[string]alignment
		Header          []string
		Data            map[string][]styledText
	}

	alignment int

	styledText struct {
		Runes []rune
		FG    []termbox.Attribute
		BG    []termbox.Attribute
	}

	machine struct {
		Name          string `json:"name"`
		User          string `json:"user"`
		Host          string `json:"host"`
		Port          string `json:"port"`
		config        *gosh.Config
		client        *ssh.Client
		Load1         measurement `json:"load1"`
		Load5         measurement `json:"load5"`
		Load15        measurement `json:"load15"`
		CPU           measurement `json:"cpu"`
		Free          measurement `json:"free"`
		Storage       measurement `json:"storage"`
		Inode         measurement `json:"inode"`
		Connections   measurement `json:"conns"`
		Uptime        measurement `json:"utime"`
		Services      measurement `json:"services"`
		Nproc         int32       `json:"nproc"`
		Fetching      bool
		GotResult     bool
		Status        int
		FetchingError string
	}

	measurement struct {
		Value   interface{}
		Warning interface{} `json:"warning"`
		Error   interface{} `json:"error"`
	}

	machineSorter struct {
		keys       []string
		keyToIndex map[string]int
	}
)

const (
	alignLeft alignment = iota
	alignCentre
	alignRight

	statusOK int = 1 << iota
	statusWarning
	statusError
	statusUnknown
)

var (
	dataFile  = flag.String("data", "", "input file")
	keyFile   = flag.String("key", "", "ssh key file")
	passFile  = flag.String("pass", "", "key password file (optional)")
	terminal  = flag.String("term", os.Getenv("TERM"), "terminal")
	cmdFile   = flag.String("cmd", "", "command file")
	sleepTime = flag.Int("t", 300, "sleep time between refresh in seconds")

	f1  string
	f2  string
	f3  string
	f4  string
	f5  string
	f6  string
	f7  string
	f8  string
	f9  string
	f10 string
	f11 string
	f12 string
)

func (s machineSorter) Len() int {
	return len(s.keys)
}

func (s machineSorter) Swap(i, j int) {
	s.keys[i], s.keys[j] = s.keys[j], s.keys[i]
	s.keyToIndex[s.keys[i]] = i
	s.keyToIndex[s.keys[j]] = j
}

func (s machineSorter) Less(i, j int) bool {
	m1 := machines[s.keys[i]]
	m2 := machines[s.keys[j]]

	if m1.Status == m2.Status {
		return sortByName(m1, m2)
	}
	return m1.Status > m2.Status
}

func sortByName(m1, m2 *machine) bool {
	return strings.ToLower(m1.Name) < strings.ToLower(m2.Name)
}

func sortByStatus(m1, m2 *machine) bool {
	if m1.Status == m2.Status {
		return sortByName(m1, m2)
	}
	return m1.Status > m2.Status
}

func getCommandsFromFile() error {
	if strings.HasPrefix(*cmdFile, "~") {
		u, err := user.Current()
		if err != nil {
			return err
		}
		*cmdFile = strings.Replace(*cmdFile, "~", u.HomeDir, 1)
	}
	data, err := ioutil.ReadFile(*cmdFile)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		strs := strings.Split(line, "=")
		if strs[0] == "F1" {
			f1 = strs[1]
		} else if strs[0] == "F2" {
			f2 = strs[1]
		} else if strs[0] == "F3" {
			f3 = strs[1]
		} else if strs[0] == "F4" {
			f4 = strs[1]
		} else if strs[0] == "F5" {
			f5 = strs[1]
		} else if strs[0] == "F6" {
			f6 = strs[1]
		} else if strs[0] == "F7" {
			f7 = strs[1]
		} else if strs[0] == "F8" {
			f8 = strs[1]
		} else if strs[0] == "F9" {
			f9 = strs[1]
		} else if strs[0] == "F10" {
			f10 = strs[1]
		} else if strs[0] == "F11" {
			f11 = strs[1]
		} else if strs[0] == "F12" {
			f12 = strs[1]
		}
	}
	return nil
}

func init() {
	if !flag.Parsed() {
		flag.Parse()
	}
	if *cmdFile != "" {
		getCommandsFromFile()
	} else {
	}
}
