package main

import (
	"flag"
	"github.com/nsf/termbox-go"
	"strings"
)

type (
	Data struct {
		Machine     string
		Load1       float32
		Load5       float32
		Load15      float32
		Free        float32
		Storage     int32
		Connections int32
		Uptime      int64
		Nproc       int32
		Fetching    bool
		GotResult   bool
	}

	Sorter struct {
		data []string
	}

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
		Name string
		User string
		IP   string
		Port string
	}
)

const (
	AlignLeft Alignment = iota
	AlignCentre
	AlignRight
)

var (
	dataFile  = flag.String("data", "", "input file")
	keyFile   = flag.String("key", "", "ssh key file")
	passFile  = flag.String("pass", "", "key password file (optional)")
	sleepTime = flag.Int("t", 300, "sleep time between refresh in seconds")
)

func (s Sorter) Len() int {
	return len(s.data)
}

func (s Sorter) Swap(i, j int) {
	s.data[i], s.data[j] = s.data[j], s.data[i]
}

func (s Sorter) Less(i, j int) bool {
	return strings.ToLower(s.data[i]) < strings.ToLower(s.data[j])
}

func init() {
	if !flag.Parsed() {
		flag.Parse()
	}
}
