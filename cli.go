package main

import (
	"fmt"
	"github.com/nsf/termbox-go"
	"os/exec"
	"time"
)

var (
	tic        TextInColumns
	errorLayer map[int]string
	hIndex     = " "
	hMachine   = "Machine"
	hLoad1     = "l1"
	hLoad5     = "l5"
	hLoad15    = "l15"
	hCPU       = "CPU"
	hFree      = "free"
	hStorage   = "/"
	hCons      = "conns"
	hUptime    = "uptime"

	headerRow      = 3
	dateRow        = 1
	dataStartRow   = 4
	startPosition  = 0
	cursorPosition = 0

	silent  bool
	showIPs bool

	selectedBg = termbox.ColorBlack | termbox.AttrBold
	selectedFg = termbox.ColorWhite | termbox.AttrBold
)

func newStyledText() StyledText {
	return StyledText{Runes: make([]rune, 0), FG: make([]termbox.Attribute, 0), BG: make([]termbox.Attribute, 0)}
}

func Init(m map[string]*Machine) {
	tic = TextInColumns{}
	errorLayer = make(map[int]string)
	tic.Header = []string{hIndex, hMachine, hLoad1, hLoad5, hLoad15, hCPU, hFree, hStorage, hCons, hUptime}
	tic.Data = make(map[string][]StyledText)
	tic.ColumnWidth = make(map[string]int)
	for _, h := range tic.Header {
		tic.Data[h] = make([]StyledText, len(m))
		tic.ColumnWidth[h] = len(h)
	}
	tic.ColumnAlignment = map[string]Alignment{
		hIndex:   AlignRight,
		hMachine: AlignRight,
		hLoad1:   AlignRight,
		hLoad5:   AlignRight,
		hLoad15:  AlignRight,
		hCPU:     AlignRight,
		hFree:    AlignRight,
		hStorage: AlignRight,
		hCons:    AlignRight,
		hUptime:  AlignLeft,
	}
	for k, _ := range m {
		if len(k) > tic.ColumnWidth[hMachine] {
			tic.ColumnWidth[hMachine] = len(k)
		}
	}
}

func drawAll() {
	for i, _ := range sortedKeys {
		formatAtIndex(i)
	}
	redraw()
}

func redraw() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	adjustStartPosition()
	drawDate()
	drawHeader()
	for i, _ := range sortedKeys {
		drawAtIndex(i, false)
	}
	termbox.Flush()
}

func drawMachine(machine string) {
	for idx, _ := range sortedKeys {
		if sortedKeys[idx] == machine {
			formatAtIndex(idx)
			drawAtIndex(idx, true)
			break
		}
	}
}

func formatAtIndex(i int) {
	d := data[sortedKeys[i]]
	d.Status = StatusOK
	formatIndex(i, d)
	if d.GotResult {
		formatLoads(i, d)
		formatCPU(i, d)
		formatFree(i, d)
		formatStorage(i, d)
		formatCons(i, d)
		formatUptime(i, d)
		delete(errorLayer, i)
	} else {
		clearInfo(i)
		errorLayer[i] = d.FetchingError
	}
	formatName(i, d)
}

func clearInfo(i int) {
	for j := 2; j < len(tic.Header); j++ {
		s := newStyledText()
		appendNoData(&s, i)
		tic.Data[tic.Header[j]][i] = s
	}
}

func appendSilent(s *StyledText, i int) {
	s.Runes = append(s.Runes, '\u00b7')
	s.FG = append(s.FG, 9)
	addBgColor(s, i)
}

func appendNoData(s *StyledText, i int) {
	s.Runes = append(s.Runes, ' ')
	s.FG = append(s.FG, termbox.ColorDefault)
	addBgColor(s, i)
}

func rowToHeader(s *StyledText, i int, header string) {
	tic.Data[header][i] = *s
	if len(s.Runes) > tic.ColumnWidth[header] {
		tic.ColumnWidth[header] = len(s.Runes)
		redraw()
	}
}

func formatIndex(i int, d *Data) {
	s := newStyledText()
	for _, r := range fmt.Sprintf("%2d", i+1) {
		s.Runes = append(s.Runes, r)
		if cursorPosition == i {
			s.FG = append(s.FG, selectedFg)
		} else {
			if d.Fetching {
				s.FG = append(s.FG, 3|termbox.AttrBold)
			} else {
				s.FG = append(s.FG, 9)
			}
		}
		addBgColor(&s, i)
	}
	tic.Data[hIndex][i] = s
}

func addBgColor(s *StyledText, i int) {
	if cursorPosition == i {
		s.BG = append(s.BG, selectedBg)
	} else {
		s.BG = append(s.BG, termbox.ColorDefault)
	}
}

func formatName(i int, d *Data) {
	s := newStyledText()
	name := d.Machine
	if showIPs {
		name = d.IP
	}
	for _, r := range name {
		s.Runes = append(s.Runes, r)
		if cursorPosition == i {
			s.FG = append(s.FG, selectedFg)
		} else {
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
		}
		addBgColor(&s, i)
	}
	tic.Data[hMachine][i] = s
}

func formatLoads(i int, d *Data) {
	rowToHeader(formatLoad(d.Load1, d, i), i, hLoad1)
	rowToHeader(formatLoad(d.Load5, d, i), i, hLoad5)
	rowToHeader(formatLoad(d.Load15, d, i), i, hLoad15)
}

func formatLoad(load float32, d *Data, i int) *StyledText {
	s := newStyledText()
	if silent && load < float32(d.Nproc)*0.8 {
		appendSilent(&s, i)
	} else {
		for _, r := range fmt.Sprintf("%.2f", load) {
			s.Runes = append(s.Runes, r)
			if cursorPosition == i {
				s.FG = append(s.FG, selectedFg)
			} else {
				if load < float32(d.Nproc)*0.8 {
					s.FG = append(s.FG, 3)
				} else if load < float32(d.Nproc) {
					s.FG = append(s.FG, 4|termbox.AttrBold)
					d.Status |= StatusWarning
				} else {
					s.FG = append(s.FG, 2|termbox.AttrBold)
					d.Status |= StatusError
				}
			}
			addBgColor(&s, i)
		}
	}
	return &s
}

func formatCPU(i int, d *Data) {
	s := newStyledText()
	if silent && d.CPU < float32(80*d.Nproc) {
		appendSilent(&s, i)
	} else {
		for _, r := range fmt.Sprintf("%.1f", d.CPU) {
			s.Runes = append(s.Runes, r)
			if cursorPosition == i {
				s.FG = append(s.FG, selectedFg)
			} else {
				if d.CPU < float32(80*d.Nproc) {
					s.FG = append(s.FG, 3)
				} else if d.CPU < float32(90*d.Nproc) {
					s.FG = append(s.FG, 4|termbox.AttrBold)
					d.Status |= StatusWarning
				} else {
					s.FG = append(s.FG, 2|termbox.AttrBold)
					d.Status |= StatusError
				}
			}
			addBgColor(&s, i)
		}
		for _, r := range fmt.Sprintf(":%d", d.Nproc) {
			s.Runes = append(s.Runes, r)
			if cursorPosition == i {
				s.FG = append(s.FG, selectedFg)
			} else {
				s.FG = append(s.FG, 9)
			}
			addBgColor(&s, i)
		}
	}
	rowToHeader(&s, i, hCPU)
}

func formatFree(i int, d *Data) {
	s := newStyledText()
	if silent && d.Free < 0.8 {
		appendSilent(&s, i)
	} else {
		for _, r := range fmt.Sprintf("%.2f", d.Free) {
			s.Runes = append(s.Runes, r)
			if cursorPosition == i {
				s.FG = append(s.FG, selectedFg)
			} else {
				if d.Free < 0.8 {
					s.FG = append(s.FG, 3)
				} else if d.Free < 0.9 {
					s.FG = append(s.FG, 4|termbox.AttrBold)
					d.Status |= StatusWarning
				} else {
					s.FG = append(s.FG, 2|termbox.AttrBold)
					d.Status |= StatusError
				}
			}
			addBgColor(&s, i)
		}
	}
	rowToHeader(&s, i, hFree)
}

func formatStorage(i int, d *Data) {
	s := newStyledText()
	if silent && d.Storage < 80 {
		appendSilent(&s, i)
	} else {
		for _, r := range fmt.Sprintf("%3d", d.Storage) {
			s.Runes = append(s.Runes, r)
			if cursorPosition == i {
				s.FG = append(s.FG, selectedFg)
			} else {
				if d.Storage < 80 {
					s.FG = append(s.FG, 3)
				} else if d.Free < 90 {
					s.FG = append(s.FG, 4|termbox.AttrBold)
					d.Status |= StatusWarning
				} else {
					s.FG = append(s.FG, 2|termbox.AttrBold)
					d.Status |= StatusError
				}
			}
			addBgColor(&s, i)
		}
	}
	rowToHeader(&s, i, hStorage)
}

func formatCons(i int, d *Data) {
	s := newStyledText()
	if silent && d.Connections < 1000 {
		appendSilent(&s, i)
	} else {
		for _, r := range fmt.Sprintf("%d", d.Connections) {
			s.Runes = append(s.Runes, r)
			if cursorPosition == i {
				s.FG = append(s.FG, selectedFg)
			} else {
				if d.Connections < 1000 {
					s.FG = append(s.FG, 3)
				} else if d.Connections < 10000 {
					s.FG = append(s.FG, 4|termbox.AttrBold)
					d.Status |= StatusWarning
				} else {
					s.FG = append(s.FG, 2|termbox.AttrBold)
					d.Status |= StatusError
				}
			}
			addBgColor(&s, i)
		}
	}
	rowToHeader(&s, i, hCons)
}

func formatUptime(i int, d *Data) {
	s := newStyledText()
	if silent {
		appendSilent(&s, i)
	} else {
		for _, r := range time.Duration(time.Duration(d.Uptime) * time.Second).String() {
			s.Runes = append(s.Runes, r)
			if cursorPosition == i {
				s.FG = append(s.FG, selectedFg)
			} else {
				if d.Uptime < 60 {
					s.FG = append(s.FG, 8)
				} else if d.Uptime < 3600 {
					s.FG = append(s.FG, 4|termbox.AttrBold)
				} else {
					s.FG = append(s.FG, 9)
				}
			}
			addBgColor(&s, i)
		}
	}
	rowToHeader(&s, i, hUptime)
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
			termbox.SetCell(position+j, headerRow, r, 9|termbox.AttrBold, termbox.ColorDefault)
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
	row := i - startPosition + dataStartRow
	bg := termbox.ColorDefault
	if cursorPosition == i {
		bg = selectedBg
	}
	for j := 0; j < w; j++ {
		termbox.SetCell(j, row, ' ', termbox.ColorDefault, bg)
	}
	currentTab := 1
	for _, he := range tic.Header {
		addRowToHeader(he, &currentTab, row, i)
	}
	if v, ok := errorLayer[i]; ok {
		fg := termbox.ColorRed
		if cursorPosition == i {
			fg = selectedFg
		}
		for j, r := range v {
			termbox.SetCell(tic.ColumnWidth[hIndex]+tic.ColumnWidth[hMachine]+j+3, row, r, fg, bg)
		}
	}
	if flush {
		termbox.Flush()
	}
}

func addRowToHeader(he string, currentTab *int, row, i int) {
	position := *currentTab
	s := tic.Data[he][i]
	if tic.ColumnAlignment[he] == AlignCentre {
		position += ((tic.ColumnWidth[he] - len(s.Runes)) / 2)
	} else if tic.ColumnAlignment[he] == AlignRight {
		position += (tic.ColumnWidth[he] - len(s.Runes))
	}
	for j := 0; j < len(s.Runes); j++ {
		termbox.SetCell(position+j, row, s.Runes[j], s.FG[j], s.BG[j])
	}
	*currentTab += tic.ColumnWidth[he] + 1
}

func adjustStartPosition() {
	_, h := termbox.Size()
	if h > (len(tic.Data[hMachine]) + dataStartRow) {
		startPosition = 0
	} else {
		if startPosition+h-dataStartRow > len(tic.Data[hMachine]) {
			startPosition = len(tic.Data[hMachine]) - (h - dataStartRow)
		}
	}
}

func openConsole() {
	name := machines[sortedKeys[cursorPosition]].Name
	user := machines[sortedKeys[cursorPosition]].config.User
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
					go func() {
						fetchTime = time.Now()
						drawDate()
						RunOnHosts()
					}()
				}
			case termbox.KeyArrowUp:
				if cursorPosition > 0 {
					if cursorPosition == startPosition {
						if startPosition > 0 {
							startPosition--
						}
					}
					cursorPosition--
					drawAll()
				}
			case termbox.KeyArrowDown:
				_, h := termbox.Size()
				if cursorPosition < len(tic.Data[hMachine])-1 {
					cursorPosition++
					if cursorPosition == startPosition+(h-dataStartRow) {
						if startPosition < len(tic.Data[hMachine]) {
							startPosition++
						}
					}
					drawAll()
				}
			case termbox.KeyEnter:
				openConsole()
			case termbox.KeyEnd:
				_, h := termbox.Size()
				cursorPosition = len(tic.Data[hMachine]) - 1
				startPosition = len(tic.Data[hMachine]) - (h - dataStartRow)
				drawAll()
			case termbox.KeyHome:
				cursorPosition = 0
				startPosition = 0
				drawAll()
			case termbox.KeyPgdn:
				_, h := termbox.Size()
				pageSize := h - dataStartRow
				dataLength := len(tic.Data[tic.Header[0]])
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
				drawAll()
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
				drawAll()
			case termbox.KeyEsc:
				break loop
			}
			switch ev.Ch {
			case 115:
				silent = !silent
				drawAll()
			case 105:
				showIPs = !showIPs
				drawAll()
			case 114:
				if !data[sortedKeys[cursorPosition]].Fetching {
					go func() {
						fetchTime = time.Now()
						drawDate()
						RunOnHost(machines[sortedKeys[cursorPosition]].Name)
					}()
				}
			}
		case termbox.EventResize:
			drawAll()
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
	drawAll()
	keyLoop()
}
