package main

import (
	"fmt"
	"github.com/nsf/termbox-go"
	"os/exec"
	"sync"
	"time"
)

var (
	tic           TextInColumns
	errorLayer    map[string]string
	headerToIndex map[string]int
	hMachine      = "Machine"
	hLoad1        = "l1"
	hLoad5        = "l5"
	hLoad15       = "l15"
	hCPU          = "CPU"
	hFree         = "free"
	hStorage      = "/"
	hInode        = "inode"
	hCons         = "conns"
	hUptime       = "uptime"

	headerRow      = 3
	dateRow        = 1
	dataStartRow   = 4
	startPosition  = 0
	cursorPosition = 0

	silent         bool
	showIPs        bool
	forceReConnect bool

	selectedBg = termbox.ColorBlack | termbox.AttrBold
	selectedFg = termbox.ColorWhite | termbox.AttrBold

	mutex sync.Mutex
)

func newStyledText() StyledText {
	return StyledText{Runes: make([]rune, 0), FG: make([]termbox.Attribute, 0), BG: make([]termbox.Attribute, 0)}
}

func Init(m map[string]*Machine) {
	tic = TextInColumns{}
	errorLayer = make(map[string]string)
	tic.Header = []string{hMachine, hLoad1, hLoad5, hLoad15, hCPU, hFree, hStorage, hInode, hCons, hUptime}
	tic.Data = make(map[string][]StyledText)
	tic.ColumnWidth = make(map[string]int)
	headerToIndex = make(map[string]int)
	for i, h := range tic.Header {
		tic.ColumnWidth[h] = len(h)
		headerToIndex[h] = i
	}
	tic.ColumnAlignment = map[string]Alignment{
		hMachine: AlignRight,
		hLoad1:   AlignRight,
		hLoad5:   AlignRight,
		hLoad15:  AlignRight,
		hCPU:     AlignRight,
		hFree:    AlignRight,
		hStorage: AlignRight,
		hInode:   AlignRight,
		hCons:    AlignRight,
		hUptime:  AlignLeft,
	}
	for k, _ := range m {
		tic.Data[k] = make([]StyledText, len(tic.Header))
		if len(k) > tic.ColumnWidth[hMachine] {
			tic.ColumnWidth[hMachine] = len(k)
		}
	}
}

func redraw() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	adjustStartPosition()
	drawDate()
	drawHeader()
	for i, _ := range sorter.keys {
		drawAtIndex(i, false)
	}
	mutex.Lock()
	termbox.Flush()
	mutex.Unlock()
}

func formatAll() {
	for _, k := range sorter.keys {
		formatMachine(k)
	}
}

func formatMachine(machine string) {
	d := data[machine]
	if d.GotResult {
		formatLoads(d)
		formatCPU(d)
		formatFree(d)
		formatStorage(d)
		formatInode(d)
		formatCons(d)
		formatUptime(d)
		delete(errorLayer, machine)
	} else {
		clearInfo(machine)
		errorLayer[machine] = d.FetchingError
	}
	formatName(d)
}

func clearInfo(machine string) {
	for j := 2; j < len(tic.Header); j++ {
		s := newStyledText()
		appendNoData(&s)
		tic.Data[machine][j] = s
	}
}

func appendSilent(s *StyledText) {
	s.Runes = append(s.Runes, '\u00b7')
	s.FG = append(s.FG, 9)
	s.BG = append(s.BG, termbox.ColorDefault)
}

func appendNoData(s *StyledText) {
	s.Runes = append(s.Runes, ' ')
	s.FG = append(s.FG, termbox.ColorDefault)
	s.BG = append(s.BG, termbox.ColorDefault)
}

func rowToHeader(s *StyledText, machine string, header string) {
	tic.Data[machine][headerToIndex[header]] = *s
	if len(s.Runes) > tic.ColumnWidth[header] {
		tic.ColumnWidth[header] = len(s.Runes)
		redraw()
	}
}

func formatName(d *Data) {
	s := newStyledText()
	name := d.Machine
	if showIPs {
		name = d.IP
	}
	for _, r := range name {
		s.Runes = append(s.Runes, r)
		if d.GotResult {
			if d.Status&StatusError > 0 {
				s.FG = append(s.FG, 2)
			} else if d.Status&StatusWarning > 0 {
				s.FG = append(s.FG, 4)
			} else {
				if silent {
					s.FG = append(s.FG, 9)
				} else {
					s.FG = append(s.FG, termbox.ColorDefault)
				}
			}
		} else {
			s.FG = append(s.FG, 9)
		}
		s.BG = append(s.BG, termbox.ColorDefault)
	}
	rowToHeader(&s, d.Machine, hMachine)
}

func formatLoads(d *Data) {
	rowToHeader(formatLoad(d.Load1, d), d.Machine, hLoad1)
	rowToHeader(formatLoad(d.Load5, d), d.Machine, hLoad5)
	rowToHeader(formatLoad(d.Load15, d), d.Machine, hLoad15)
}

func formatLoad(load float32, d *Data) *StyledText {
	s := newStyledText()
	if silent && load < float32(d.Nproc)*0.8 {
		appendSilent(&s)
	} else {
		for _, r := range fmt.Sprintf("%.2f", load) {
			s.Runes = append(s.Runes, r)
			if load < float32(d.Nproc)*0.8 {
				s.FG = append(s.FG, 3)
			} else if load < float32(d.Nproc) {
				s.FG = append(s.FG, 4|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 2|termbox.AttrBold)
			}
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	return &s
}

func formatCPU(d *Data) {
	s := newStyledText()
	if silent && d.CPU < float32(80*d.Nproc) {
		appendSilent(&s)
	} else {
		for _, r := range fmt.Sprintf("%.1f", d.CPU) {
			s.Runes = append(s.Runes, r)
			if d.CPU < float32(80*d.Nproc) {
				s.FG = append(s.FG, 3)
			} else if d.CPU < float32(90*d.Nproc) {
				s.FG = append(s.FG, 4|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 2|termbox.AttrBold)
			}
			s.BG = append(s.BG, termbox.ColorDefault)
		}
		for _, r := range fmt.Sprintf(":%d", d.Nproc) {
			s.Runes = append(s.Runes, r)
			s.FG = append(s.FG, 9)
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	rowToHeader(&s, d.Machine, hCPU)
}

func formatFree(d *Data) {
	s := newStyledText()
	if silent && d.Free < 0.8 {
		appendSilent(&s)
	} else {
		for _, r := range fmt.Sprintf("%.2f", d.Free) {
			s.Runes = append(s.Runes, r)
			if d.Free < 0.8 {
				s.FG = append(s.FG, 3)
			} else if d.Free < 0.9 {
				s.FG = append(s.FG, 4|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 2|termbox.AttrBold)
			}
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	rowToHeader(&s, d.Machine, hFree)
}

func formatStorage(d *Data) {
	s := newStyledText()
	if silent && d.Storage < 80 {
		appendSilent(&s)
	} else {
		for _, r := range fmt.Sprintf("%3d", d.Storage) {
			s.Runes = append(s.Runes, r)
			if d.Storage < 80 {
				s.FG = append(s.FG, 3)
			} else if d.Storage < 90 {
				s.FG = append(s.FG, 4|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 2|termbox.AttrBold)
			}
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	rowToHeader(&s, d.Machine, hStorage)
}

func formatInode(d *Data) {
	s := newStyledText()
	if silent && d.Inode < 80 {
		appendSilent(&s)
	} else {
		for _, r := range fmt.Sprintf("%3d", d.Inode) {
			s.Runes = append(s.Runes, r)
			if d.Inode < 80 {
				s.FG = append(s.FG, 3)
			} else if d.Inode < 90 {
				s.FG = append(s.FG, 4|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 2|termbox.AttrBold)
			}
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	rowToHeader(&s, d.Machine, hInode)
}

func formatCons(d *Data) {
	s := newStyledText()
	if silent && d.Connections < 10000 {
		appendSilent(&s)
	} else {
		for _, r := range fmt.Sprintf("%d", d.Connections) {
			s.Runes = append(s.Runes, r)
			if d.Connections < 10000 {
				s.FG = append(s.FG, 3)
			} else if d.Connections < 50000 {
				s.FG = append(s.FG, 4|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 2|termbox.AttrBold)
			}
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	rowToHeader(&s, d.Machine, hCons)
}

func formatUptime(d *Data) {
	s := newStyledText()
	if silent {
		appendSilent(&s)
	} else {
		for _, r := range time.Duration(time.Duration(d.Uptime) * time.Second).String() {
			s.Runes = append(s.Runes, r)
			if d.Uptime < 60 {
				s.FG = append(s.FG, 8)
			} else if d.Uptime < 3600 {
				s.FG = append(s.FG, 4|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 9)
			}
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	rowToHeader(&s, d.Machine, hUptime)
}

func drawHeader() {
	currentTab := 1
	for _, h := range tic.Header {
		position := currentTab
		if tic.ColumnAlignment[h] == AlignCentre {
			position += ((tic.ColumnWidth[h] - len(h)) / 2)
		} else if tic.ColumnAlignment[h] == AlignRight {
			position += (tic.ColumnWidth[h] - len(h))
		}
		for j, r := range h {
			termbox.SetCell(3+position+j, headerRow, r, 9|termbox.AttrBold, termbox.ColorDefault)
		}
		currentTab += tic.ColumnWidth[h] + 1
	}
}

func drawDate() {
	w := 1
	for _, h := range tic.Header {
		w += tic.ColumnWidth[h] + 1
	}
	d := fetchTime.Format(time.RFC1123)
	for j, r := range d {
		termbox.SetCell((w-len(d))/2+j, dateRow, r, 8, termbox.ColorDefault)
	}
}

func drawAtIndex(i int, flush bool) {
	w, h := termbox.Size()
	if i < startPosition || i > startPosition+h-dataStartRow {
		return
	}
	name := sorter.keys[i]
	row := i - startPosition + dataStartRow
	bg := termbox.ColorDefault
	var indexFg termbox.Attribute
	indexFg = 9
	if data[name].Fetching {
		indexFg = termbox.ColorGreen | termbox.AttrBold
	}
	selected := cursorPosition == i
	if selected {
		indexFg = selectedFg
		bg = selectedBg
	}
	for j := 0; j < w; j++ {
		termbox.SetCell(j, row, ' ', termbox.ColorDefault, bg)
	}
	currentTab := 1
	index := fmt.Sprintf("%2d", i+1)
	for j, r := range index {
		termbox.SetCell(currentTab+j, row, r, indexFg, bg)
	}
	currentTab += len(index) + 1
	for heidx, he := range tic.Header {
		position := currentTab
		s := tic.Data[name][heidx]
		if tic.ColumnAlignment[he] == AlignCentre {
			position += ((tic.ColumnWidth[he] - len(s.Runes)) / 2)
		} else if tic.ColumnAlignment[he] == AlignRight {
			position += (tic.ColumnWidth[he] - len(s.Runes))
		}
		for j := 0; j < len(s.Runes); j++ {
			fg := s.FG[j]
			bg := s.BG[j]
			if selected {
				fg = selectedFg
				bg = selectedBg
			}
			termbox.SetCell(position+j, row, s.Runes[j], fg, bg)
		}
		currentTab += tic.ColumnWidth[he] + 1
	}

	if v, ok := errorLayer[name]; ok {
		fg := termbox.ColorRed
		if selected {
			fg = selectedFg
		}
		for j, r := range v {
			termbox.SetCell(len(index)+tic.ColumnWidth[hMachine]+j+3, row, r, fg, bg)
		}
	}
	if flush {
		termbox.Flush()
	}
}

func adjustStartPosition() {
	_, h := termbox.Size()
	if h > (len(tic.Data) + dataStartRow) {
		startPosition = 0
	} else {
		if startPosition+h-dataStartRow > len(tic.Data) {
			startPosition = len(tic.Data) - (h - dataStartRow)
		}
	}
}

func openConsole() {
	name := machines[sorter.keys[cursorPosition]].Name
	user := machines[sorter.keys[cursorPosition]].config.User
	cmd := exec.Command("urxvt", "-e", "ssh", fmt.Sprintf("%s@%s", user, name))
	go func() {
		err := cmd.Run()
		if err != nil {
			fmt.Println(err.Error())
		}
	}()
}

func keyLoop() {
loop:
	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyCtrlR:
				if !running {
					go func(forceReConnect bool) {
						fetchTime = time.Now()
						drawDate()
						RunOnHosts(forceReConnect)
					}(forceReConnect)
					forceReConnect = false
				}
			case termbox.KeyCtrlF:
				forceReConnect = true
			case termbox.KeyArrowUp:
				if cursorPosition > 0 {
					if cursorPosition == startPosition {
						if startPosition > 0 {
							startPosition--
						}
					}
					cursorPosition--
					redraw()
				}
			case termbox.KeyArrowDown:
				_, h := termbox.Size()
				if cursorPosition < len(tic.Data)-1 {
					cursorPosition++
					if cursorPosition == startPosition+(h-dataStartRow) {
						if startPosition < len(tic.Data) {
							startPosition++
						}
					}
					redraw()
				}
			case termbox.KeyEnter:
				openConsole()
			case termbox.KeyEnd:
				_, h := termbox.Size()
				cursorPosition = len(tic.Data) - 1
				startPosition = len(tic.Data) - (h - dataStartRow)
				redraw()
			case termbox.KeyHome:
				cursorPosition = 0
				startPosition = 0
				redraw()
			case termbox.KeyPgdn:
				_, h := termbox.Size()
				pageSize := h - dataStartRow
				dataLength := len(tic.Data)
				if cursorPosition+pageSize < dataLength {
					cursorPosition += pageSize
				} else {
					cursorPosition = dataLength - 1
				}
				if startPosition+pageSize < dataLength-pageSize {
					startPosition += pageSize
				} else {
					startPosition = dataLength - pageSize
				}
				redraw()
			case termbox.KeyPgup:
				_, h := termbox.Size()
				pageSize := h - dataStartRow
				if cursorPosition-pageSize < 0 {
					cursorPosition = 0
					startPosition = 0
				} else {
					cursorPosition -= pageSize
					if cursorPosition < pageSize {
						startPosition = 0
					} else {
						startPosition = cursorPosition
					}
				}
				redraw()
			case termbox.KeyEsc:
				break loop
			}
			switch ev.Ch {
			case 115: // s
				silent = !silent
				formatAll()
				redraw()
			case 105: // i
				showIPs = !showIPs
				formatAll()
				redraw()
			case 114: // r
				if !data[sorter.keys[cursorPosition]].Fetching {
					go func(forceReConnect bool) {
						fetchTime = time.Now()
						drawDate()
						RunOnHost(machines[sorter.keys[cursorPosition]].Name, forceReConnect)
					}(forceReConnect)
					forceReConnect = false
				}
			}
		case termbox.EventResize:
			redraw()
		}
	}
}

func runCli() {
	err := termbox.Init()
	if err != nil {
		panic(err)
	}
	defer termbox.Close()
	termbox.SetOutputMode(termbox.Output256)
	redraw()
	keyLoop()
}
