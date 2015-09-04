package main

import (
	"flag"
	"fmt"
	"github.com/madislohmus/gosh"
	"github.com/nsf/termbox-go"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"os"
	"os/user"
	"strings"
)

type (
	TextInColumns struct {
		ColumnWidth     map[string]int
		ColumnAlignment map[string]Alignment
		Header          []string
		Data            map[string][]StyledText
	}

	Alignment int

	StyledText struct {
		Runes []rune
		FG    []termbox.Attribute
		BG    []termbox.Attribute
	}

	Machine struct {
		Name          string `json:"name"`
		User          string `json:"user"`
		Host          string `json:"host"`
		Port          string `json:"port"`
		config        *gosh.Config
		client        *ssh.Client
		Load1         Measurement `json:"load1"`
		Load5         Measurement `json:"load5"`
		Load15        Measurement `json:"load15"`
		CPU           Measurement `json:"cpu"`
		Free          Measurement `json:"free"`
		Storage       Measurement `json:"storage"`
		Inode         Measurement `json:"inode"`
		Connections   Measurement `json:"conns"`
		Uptime        int64       `json:"utime"`
		Nproc         int32       `json:"nproc"`
		Fetching      bool
		GotResult     bool
		Status        int
		FetchingError string
	}

	Measurement struct {
		Value   interface{}
		Warning interface{} `json:"warning"`
		Error   interface{} `json:"error"`
	}

	Sorter struct {
		keys       []string
		keyToIndex map[string]int
	}
)

const (
	AlignLeft Alignment = iota
	AlignCentre
	AlignRight

	StatusOK int = 1 << iota
	StatusWarning
	StatusError
	StatusUnknown
)

var (
	dataFile  = flag.String("data", "", "input file")
	keyFile   = flag.String("key", "", "ssh key file")
	passFile  = flag.String("pass", "", "key password file (optional)")
	terminal  = flag.String("term", os.Getenv("TERM"), "terminal")
	cmdFile   = flag.String("cmd", "", "command file")
	sleepTime = flag.Int("t", 300, "sleep time between refresh in seconds")

	F1  string
	F2  string
	F3  string
	F4  string
	F5  string
	F6  string
	F7  string
	F8  string
	F9  string
	F10 string
	F11 string
	F12 string
)

func (s Sorter) Len() int {
	return len(s.keys)
}

func (s Sorter) Swap(i, j int) {
	s.keys[i], s.keys[j] = s.keys[j], s.keys[i]
	s.keyToIndex[s.keys[i]] = i
	s.keyToIndex[s.keys[j]] = j
}

func (s Sorter) Less(i, j int) bool {
	m1 := machines[s.keys[i]]
	m2 := machines[s.keys[j]]

	if m1.Status == m2.Status {
		return sortByName(m1, m2)
	} else {
		return m1.Status > m2.Status
	}

}

func sortByName(m1, m2 *Machine) bool {
	return strings.ToLower(m1.Name) < strings.ToLower(m2.Name)
}

func sortByStatus(m1, m2 *Machine) bool {
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
			F1 = strs[1]
		} else if strs[0] == "F2" {
			F2 = strs[1]
		} else if strs[0] == "F3" {
			F3 = strs[1]
		} else if strs[0] == "F4" {
			F4 = strs[1]
		} else if strs[0] == "F5" {
			F5 = strs[1]
		} else if strs[0] == "F6" {
			F6 = strs[1]
		} else if strs[0] == "F7" {
			F7 = strs[1]
		} else if strs[0] == "F8" {
			F8 = strs[1]
		} else if strs[0] == "F9" {
			F9 = strs[1]
		} else if strs[0] == "F10" {
			F10 = strs[1]
		} else if strs[0] == "F11" {
			F11 = strs[1]
		} else if strs[0] == "F12" {
			F12 = strs[1]
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
