package main

import (
	"fmt"
	"github.com/nsf/termbox-go"
	"math"
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
		hUptime:  AlignRight,
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
	d := machines[machine]
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
		if len(d.FetchingError) > 0 {
			errorLayer[machine] = d.FetchingError
		}
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

func formatName(d *Machine) {
	s := newStyledText()
	name := d.Name
	if showIPs {
		name = d.config.Host
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
	rowToHeader(&s, d.Name, hMachine)
}

func formatLoads(d *Machine) {
	rowToHeader(formatLoad(d.Load1, d), d.Name, hLoad1)
	rowToHeader(formatLoad(d.Load5, d), d.Name, hLoad5)
	rowToHeader(formatLoad(d.Load15, d), d.Name, hLoad15)
}

func formatLoad(load Measurement, d *Machine) *StyledText {
	s := newStyledText()
	lv := load.Value.(float32)
	if silent && lv < float32(d.Nproc)*0.8 {
		appendSilent(&s)
	} else {
		for _, r := range fmt.Sprintf("%.2f", lv) {
			s.Runes = append(s.Runes, r)
			if lv < float32(d.Nproc)*0.8 {
				s.FG = append(s.FG, 3)
			} else if lv < float32(d.Nproc) {
				s.FG = append(s.FG, 4|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 2|termbox.AttrBold)
			}
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	return &s
}

func formatCPU(d *Machine) {
	s := newStyledText()
	cpu := d.CPU.Value.(float32)
	status := getCPUStatus(d)
	if silent && (status == StatusOK) {
		appendSilent(&s)
	} else {
		for _, r := range fmt.Sprintf("%.1f", cpu) {
			s.Runes = append(s.Runes, r)
			if status == StatusOK {
				s.FG = append(s.FG, 3)
			} else if status == StatusWarning {
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
	rowToHeader(&s, d.Name, hCPU)
}

func formatFree(d *Machine) {
	s := newStyledText()
	free := d.Free.Value.(float32)
	status := getFreeStatus(d)
	if silent && (status == StatusOK) {
		appendSilent(&s)
	} else {
		for _, r := range fmt.Sprintf("%.2f", free) {
			s.Runes = append(s.Runes, r)
			if status == StatusOK {
				s.FG = append(s.FG, 3)
			} else if status == StatusWarning {
				s.FG = append(s.FG, 4|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 2|termbox.AttrBold)
			}
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	rowToHeader(&s, d.Name, hFree)
}

func formatStorage(d *Machine) {
	s := newStyledText()
	storage := d.Storage.Value.(int32)
	status := getStorageStatus(d)
	if silent && (status == StatusOK) {
		appendSilent(&s)
	} else {
		for _, r := range fmt.Sprintf("%3d", storage) {
			s.Runes = append(s.Runes, r)
			if status == StatusOK {
				s.FG = append(s.FG, 3)
			} else if status == StatusWarning {
				s.FG = append(s.FG, 4|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 2|termbox.AttrBold)
			}
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	rowToHeader(&s, d.Name, hStorage)
}

func formatInode(d *Machine) {
	s := newStyledText()
	inode := d.Inode.Value.(int32)
	status := getInodeStatus(d)
	if silent && (status == StatusOK) {
		appendSilent(&s)
	} else {
		for _, r := range fmt.Sprintf("%3d", inode) {
			s.Runes = append(s.Runes, r)
			if status == StatusOK {
				s.FG = append(s.FG, 3)
			} else if status == StatusWarning {
				s.FG = append(s.FG, 4|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 2|termbox.AttrBold)
			}
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	rowToHeader(&s, d.Name, hInode)
}

func formatCons(d *Machine) {
	s := newStyledText()
	conns := d.Connections.Value.(int32)
	status := getConnectionsStatus(d)
	if silent && (status == StatusOK) {
		appendSilent(&s)
	} else {
		for _, r := range fmt.Sprintf("%d", conns) {
			s.Runes = append(s.Runes, r)
			if status == StatusOK {
				s.FG = append(s.FG, 3)
			} else if status == StatusWarning {
				s.FG = append(s.FG, 4|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 2|termbox.AttrBold)
			}
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	rowToHeader(&s, d.Name, hCons)
}

func formatUptime(d *Machine) {
	s := newStyledText()
	if silent {
		appendSilent(&s)
	} else {
		for _, r := range formatDuration(d.Uptime) {
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
	rowToHeader(&s, d.Name, hUptime)
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

func formatDuration(duration int64) string {
	h := int64(math.Floor(float64(duration / 3600.0)))
	m := int64(math.Floor(float64(float64(duration)-float64(3600*h)) / 60.0))
	s := int64(math.Floor((float64(duration) - float64(3600*h) - float64(60*m))))
	var hs, ms = "", ""
	if h > 0 {
		hs = fmt.Sprintf("%02d:", h)
	}
	if h > 0 || m > 0 {
		ms = fmt.Sprintf("%02d:", m)
	}
	return fmt.Sprintf("%s%s%02d", hs, ms, s)
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
	if machines[name].Fetching {
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
		label := v
		if silent {
			label = "E"
		}
		for j, r := range label {
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
				if !machines[sorter.keys[cursorPosition]].Fetching {
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
