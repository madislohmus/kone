package main

import (
	"flag"
	"github.com/madislohmus/gosh"
	"github.com/nsf/termbox-go"
	"golang.org/x/crypto/ssh"
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
	sleepTime = flag.Int("t", 300, "sleep time between refresh in seconds")
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

func init() {
	if !flag.Parsed() {
		flag.Parse()
	}
}
