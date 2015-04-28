package main

import (
	"fmt"
	"github.com/nsf/termbox-go"
	"time"
)

var (
	tic      TextInColumns
	hIndex   = " "
	hMachine = "Machine"
	hLoad1   = "l1"
	hLoad5   = "l5"
	hLoad15  = "l15"
	hCPU     = "CPU"
	hFree    = "free"
	hStorage = "df /"
	hCons    = "conns"
	hUptime  = "uptime"

	headerRow     = 3
	dateRow       = 1
	dataStartRow  = 4
	startPosition = 0

	silent  bool
	showIPs bool
)

func newStyledText() StyledText {
	return StyledText{Runes: make([]rune, 0), FG: make([]termbox.Attribute, 0), BG: make([]termbox.Attribute, 0)}
}

func Init(m map[string]Machine) {
	tic = TextInColumns{}
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
	formatLoads(i, d)
	formatCPU(i, d)
	formatFree(i, d)
	formatStorage(i, d)
	formatCons(i, d)
	formatUptime(i, d)
	formatMachine(i, d)
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
		if d.Fetching {
			s.FG = append(s.FG, 3|termbox.AttrBold)
		} else {
			s.FG = append(s.FG, 9)
		}
		s.BG = append(s.BG, termbox.ColorDefault)
	}
	tic.Data[hIndex][i] = s
}

func formatMachine(i int, d *Data) {
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
	tic.Data[hMachine][i] = s
}

func formatLoads(i int, d *Data) {
	rowToHeader(formatLoad(d.Load1, d), i, hLoad1)
	rowToHeader(formatLoad(d.Load5, d), i, hLoad5)
	rowToHeader(formatLoad(d.Load15, d), i, hLoad15)
}

func formatLoad(load float32, d *Data) *StyledText {
	s := newStyledText()
	if d.GotResult {
		if silent && load < float32(d.Nproc)*0.8 {
			appendSilent(&s)
		} else {
			for _, r := range fmt.Sprintf("%.2f", load) {
				s.Runes = append(s.Runes, r)
				if load < float32(d.Nproc)*0.8 {
					s.FG = append(s.FG, 3)
				} else if load < float32(d.Nproc) {
					s.FG = append(s.FG, 4|termbox.AttrBold)
					d.Status |= StatusWarning
				} else {
					s.FG = append(s.FG, 2|termbox.AttrBold)
					d.Status |= StatusError
				}
				s.BG = append(s.BG, termbox.ColorDefault)
			}
		}
	} else {
		appendNoData(&s)
	}
	return &s
}

func formatCPU(i int, d *Data) {
	s := newStyledText()
	if d.GotResult {
		if silent && d.CPU < float32(80*d.Nproc) {
			appendSilent(&s)
		} else {
			for _, r := range fmt.Sprintf("%.1f", d.CPU) {
				s.Runes = append(s.Runes, r)
				if d.CPU < float32(80*d.Nproc) {
					s.FG = append(s.FG, 3)
				} else if d.CPU < float32(90*d.Nproc) {
					s.FG = append(s.FG, 4|termbox.AttrBold)
					d.Status |= StatusWarning
				} else {
					s.FG = append(s.FG, 2|termbox.AttrBold)
					d.Status |= StatusError
				}
				s.BG = append(s.BG, termbox.ColorDefault)
			}
		}
	} else {
		appendNoData(&s)
	}
	rowToHeader(&s, i, hCPU)
}

func formatFree(i int, d *Data) {
	s := newStyledText()
	if d.GotResult {
		if silent && d.Free < 0.8 {
			appendSilent(&s)
		} else {
			for _, r := range fmt.Sprintf("%.2f", d.Free) {
				s.Runes = append(s.Runes, r)
				if d.Free < 0.8 {
					s.FG = append(s.FG, 3)
				} else if d.Free < 0.9 {
					s.FG = append(s.FG, 4|termbox.AttrBold)
					d.Status |= StatusWarning
				} else {
					s.FG = append(s.FG, 2|termbox.AttrBold)
					d.Status |= StatusError
				}
				s.BG = append(s.BG, termbox.ColorDefault)
			}
		}
	} else {
		appendNoData(&s)
	}
	rowToHeader(&s, i, hFree)
}

func formatStorage(i int, d *Data) {
	s := newStyledText()
	if d.GotResult {
		if silent && d.Storage < 80 {
			appendSilent(&s)
		} else {
			for _, r := range fmt.Sprintf("%3d", d.Storage) {
				s.Runes = append(s.Runes, r)
				if d.Storage < 80 {
					s.FG = append(s.FG, 3)
				} else if d.Free < 90 {
					s.FG = append(s.FG, 4|termbox.AttrBold)
					d.Status |= StatusWarning
				} else {
					s.FG = append(s.FG, 2|termbox.AttrBold)
					d.Status |= StatusError
				}
				s.BG = append(s.BG, termbox.ColorDefault)
			}
		}
	} else {
		appendNoData(&s)
	}
	rowToHeader(&s, i, hStorage)
}

func formatCons(i int, d *Data) {
	s := newStyledText()
	if d.GotResult {
		if silent && d.Connections < 1000 {
			appendSilent(&s)
		} else {
			for _, r := range fmt.Sprintf("%d", d.Connections) {
				s.Runes = append(s.Runes, r)
				if d.Connections < 1000 {
					s.FG = append(s.FG, 3)
				} else if d.Connections < 10000 {
					s.FG = append(s.FG, 4|termbox.AttrBold)
					d.Status |= StatusWarning
				} else {
					s.FG = append(s.FG, 2|termbox.AttrBold)
					d.Status |= StatusError
				}
				s.BG = append(s.BG, termbox.ColorDefault)
			}
		}
	} else {
		appendNoData(&s)
	}
	rowToHeader(&s, i, hCons)
}

func formatUptime(i int, d *Data) {
	s := newStyledText()
	if d.GotResult {
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
	} else {
		appendNoData(&s)
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
	for j := 0; j < w; j++ {
		termbox.SetCell(j, i+dataStartRow, ' ', termbox.ColorDefault, termbox.ColorDefault)
	}
	currentTab := 1
	for _, he := range tic.Header {
		for c := startPosition; c < (startPosition+h-dataStartRow) && c < len(tic.Data[he]); c++ {
			position := currentTab
			row := c + dataStartRow - startPosition
			s := tic.Data[he][c]
			if tic.ColumnAlignment[he] == AlignCentre {
				position += ((tic.ColumnWidth[he] - len(s.Runes)) / 2)
			} else if tic.ColumnAlignment[he] == AlignRight {
				position += (tic.ColumnWidth[he] - len(s.Runes))
			}
			for j := 0; j < len(s.Runes); j++ {
				termbox.SetCell(position+j, row, s.Runes[j], s.FG[j], s.BG[j])
			}
		}
		currentTab += tic.ColumnWidth[he] + 1
	}
	if flush {
		termbox.Flush()
	}
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

func runCli() {
	err := termbox.Init()
	if err != nil {
		panic(err)
	}
	defer termbox.Close()
	termbox.SetOutputMode(termbox.Output256)

	drawAll()

loop:
	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyCtrlR:
				if !running {
					go func() {
						drawDate()
						runAllHosts(command)
					}()
				}
			case termbox.KeyArrowUp:
				_, h := termbox.Size()
				if h < len(tic.Data[hMachine])+dataStartRow {
					if startPosition > 0 {
						startPosition--
						drawAll()
					}
				}
			case termbox.KeyArrowDown:
				_, h := termbox.Size()
				if h < len(tic.Data[hMachine])+dataStartRow-startPosition {
					if startPosition < len(tic.Data[hMachine]) {
						startPosition++
						drawAll()
					}
				}
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
			}
		case termbox.EventResize:
			drawAll()
		}
	}
}
