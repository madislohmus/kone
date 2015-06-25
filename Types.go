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
		Name          string
		config        *gosh.Config
		client        *ssh.Client
		Load1         Measurement
		Load5         Measurement
		Load15        Measurement
		CPU           Measurement
		Free          Measurement
		Storage       Measurement
		Inode         Measurement
		Connections   Measurement
		Uptime        int64
		Nproc         int32
		Fetching      bool
		GotResult     bool
		Status        int
		FetchingError string
	}

	Measurement struct {
		Value        interface{}
		WarningLevel interface{}
		ErrorLevel   interface{}
	}

	Sorter struct {
		keys []string
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
