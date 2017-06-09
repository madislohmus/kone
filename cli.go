package main

import (
	"fmt"
	"math"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/nsf/termbox-go"
)

const (
	headerRow    = 3
	dateRow      = 1
	dataStartRow = 4

	hMachine  = "Machine"
	hLoad     = "load"
	hCPU      = "CPU"
	hFree     = "free"
	hStorage  = "storage"
	hInode    = "inode"
	hCons     = "conns"
	hUptime   = "uptime"
	hServices = "services"
)

var (
	tic           textInColumns
	errorLayer    map[string]string
	headerToIndex map[string]int

	startPosition    = 0
	cursorPosition   = 0
	matchingCount    = 0
	matchingMachines = make(map[string]bool)

	silent         bool
	showIPs        bool
	forceReConnect bool
	search         bool

	selectedBg = termbox.ColorBlack | termbox.AttrBold
	selectedFg = termbox.ColorWhite | termbox.AttrBold

	errorLayerMutex  sync.Mutex
	redrawMutex      sync.Mutex
	columnWidthMutex sync.Mutex
	searchString     string
	indexFormat      string

	smallCircle = '\u00b7'
)

func newStyledText() styledText {
	return styledText{Runes: make([]rune, 0), FG: make([]termbox.Attribute, 0), BG: make([]termbox.Attribute, 0)}
}

func initMachines(m map[string]*machine) {
	tic = textInColumns{}
	errorLayer = make(map[string]string)
	tic.Header = []string{hMachine, hLoad, hCPU, hFree, hStorage, hInode, hCons, hUptime, hServices}
	tic.Data = make(map[string][]styledText)
	tic.ColumnWidth = make(map[string]int)
	headerToIndex = make(map[string]int)
	for i, h := range tic.Header {
		name := h
		putToColumnWidthMap(h, len(name))
		headerToIndex[h] = i
	}
	tic.ColumnAlignment = map[string]alignment{
		hMachine:  alignRight,
		hLoad:     alignCentre,
		hCPU:      alignRight,
		hFree:     alignRight,
		hStorage:  alignRight,
		hInode:    alignRight,
		hCons:     alignRight,
		hUptime:   alignRight,
		hServices: alignLeft,
	}
	for k := range m {
		tic.Data[k] = make([]styledText, len(tic.Header))
		if len(k) > getFromColumnWidthMap(hMachine) {
			putToColumnWidthMap(hMachine, len(k))
		}
		matchingMachines[k] = false
	}
	indexFormat = "%" + fmt.Sprintf("%d", len(fmt.Sprintf("%d", len(m)))) + "d"
}

func redraw() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	adjustStartPosition()
	drawDate()
	drawHeader()
	drawStatusBar()
	if search && len(searchString) > 0 {
		i := 0
		for _, k := range sorter.keys {
			if matchingMachines[k] {
				drawAtIndex(i, k, false)
				i++
			}
		}
	} else {
		for i, k := range sorter.keys {
			drawAtIndex(i, k, false)
		}
	}
	termbox.Flush()
}

func formatAll() {
	for _, k := range sorter.keys {
		formatMachine(k)
	}
}

func formatMachine(machine string) {
	d := machines[machine]
	if d.GotResult {
		formatLoad(d)
		formatCPU(d)
		formatFree(d)
		formatStorage(d)
		formatInode(d)
		formatCons(d)
		formatUptime(d)
		formatServices(d)
		errorLayerMutex.Lock()
		delete(errorLayer, machine)
		errorLayerMutex.Unlock()
	} else {
		clearInfo(machine)
		if len(d.FetchingError) > 0 {
			errorLayerMutex.Lock()
			errorLayer[machine] = d.FetchingError
			errorLayerMutex.Unlock()
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

func appendSilent(s *styledText) {
	s.Runes = append(s.Runes, smallCircle)
	s.FG = append(s.FG, 9)
	s.BG = append(s.BG, termbox.ColorDefault)
}

func appendNoData(s *styledText) {
	s.Runes = append(s.Runes, ' ')
	s.FG = append(s.FG, termbox.ColorDefault)
	s.BG = append(s.BG, termbox.ColorDefault)
}

func rowToHeader(s *styledText, machine string, header string) {
	tic.Data[machine][headerToIndex[header]] = *s
	if len(s.Runes) > getFromColumnWidthMap(header) {
		putToColumnWidthMap(header, len(s.Runes))
		sendRedrawRequest()
	}
}

func formatName(d *machine) {
	s := newStyledText()
	name := d.Name
	if showIPs {
		name = name + " (" + d.config.Host + ")"
	}
	idx := strings.Index(strings.ToLower(name), strings.ToLower(searchString))
	if idx > -1 {
		if !matchingMachines[d.Name] {
			matchingMachines[d.Name] = true
			matchingCount++
		}
	}
	for i, r := range name {
		s.Runes = append(s.Runes, r)
		if d.GotResult {
			if search && len(searchString) > 0 && idx > -1 && i >= idx && i < idx+len(searchString) {
				s.FG = append(s.FG, termbox.ColorBlack)
			} else if d.Status&statusError > 0 {
				s.FG = append(s.FG, 2)
			} else if d.Status&statusWarning > 0 {
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
		if search && len(searchString) > 0 && idx > -1 && i >= idx && i < idx+len(searchString) {
			s.BG = append(s.BG, termbox.ColorYellow)
		} else {
			s.BG = append(s.BG, termbox.ColorDefault)
		}
	}
	rowToHeader(&s, d.Name, hMachine)
}

func formatLoad(d *machine) {
	s := newStyledText()
	loads := []measurement{d.Load1, d.Load5, d.Load15}
	for i, load := range loads {
		status := getLoadStatus(d, load)
		formatStr := "%.2f"
		if i+1 < len(loads) {
			formatStr += " "
		}
		formatText(fmt.Sprintf(formatStr, load.Value.(float32)), status, &s)
	}
	rowToHeader(&s, d.Name, hLoad)
}

func formatCPU(d *machine) {
	s := newStyledText()
	status := getCPUStatus(d)
	formatText(fmt.Sprintf("%.1f", d.CPU.Value.(float32)), status, &s)
	for _, r := range fmt.Sprintf(":%d", d.Nproc) {
		if silent {
			s.Runes = append(s.Runes, ' ')
		} else {
			s.Runes = append(s.Runes, r)
		}
		s.FG = append(s.FG, 9)
		s.BG = append(s.BG, termbox.ColorDefault)
	}
	rowToHeader(&s, d.Name, hCPU)
}

func formatFree(d *machine) {
	s := newStyledText()
	status := getFreeStatus(d)
	formatText(fmt.Sprintf("%.2f", d.Free.Value.(float32)), status, &s)
	rowToHeader(&s, d.Name, hFree)
}

func formatStorage(d *machine) {
	s := newStyledText()
	warn, ok := d.Storage.Warning.(float64)
	if !ok {
		warn = 80
	}
	err, ok := d.Storage.Error.(float64)
	if !ok {
		err = 90
	}
	for _, datum := range d.Storage.Value.([]int32) {
		status := getSingleStorageStatus(datum, warn, err)
		formatText(fmt.Sprintf("%3d", datum), status, &s)
	}

	rowToHeader(&s, d.Name, hStorage)
}

func formatInode(d *machine) {
	s := newStyledText()
	warn, ok := d.Inode.Warning.(float64)
	if !ok {
		warn = 80
	}
	err, ok := d.Inode.Error.(float64)
	if !ok {
		err = 90
	}
	for _, datum := range d.Inode.Value.([]int32) {
		status := getSingleStorageStatus(datum, warn, err)
		formatText(fmt.Sprintf("%3d", datum), status, &s)
	}
	rowToHeader(&s, d.Name, hInode)
}

func formatCons(d *machine) {
	s := newStyledText()
	status := getConnectionsStatus(d)
	formatText(fmt.Sprintf("%d", d.Connections.Value.(int32)), status, &s)
	rowToHeader(&s, d.Name, hCons)
}

func formatUptime(d *machine) {
	s := newStyledText()
	status := getUptimeStatus(d)
	utime := formatDuration(d.Uptime.Value.(int64))
	for i, r := range utime {
		if silent && status == statusOK {
			if i == len(utime)-1 {
				s.Runes = append(s.Runes, smallCircle)
			} else {
				s.Runes = append(s.Runes, ' ')
			}
		} else {
			s.Runes = append(s.Runes, r)
		}
		if status == statusOK {
			if d.Uptime.Value.(int64) < 24*60*60 && !silent {
				s.FG = append(s.FG, termbox.ColorDefault)
			} else {
				s.FG = append(s.FG, 9)
			}
		} else if status == statusWarning {
			s.FG = append(s.FG, 4|termbox.AttrBold)
		} else {
			s.FG = append(s.FG, 2|termbox.AttrBold)
		}
		s.BG = append(s.BG, termbox.ColorDefault)
	}
	rowToHeader(&s, d.Name, hUptime)
}

func formatServices(d *machine) {
	s := newStyledText()
	if d.Services.Value != nil {
		statuses := [4]int{statusOK, statusUnknown, statusWarning, statusError}
		for i, datum := range d.Services.Value.([4]int32) {
			if datum == 0 {
				for i, r := range " 0" {
					if silent {
						if i == len(" 0")-1 {
							s.Runes = append(s.Runes, smallCircle)
						} else {
							s.Runes = append(s.Runes, ' ')
						}
					} else {
						s.Runes = append(s.Runes, r)
					}
					s.FG = append(s.FG, 9)
					s.BG = append(s.BG, termbox.ColorDefault)
				}
			} else {
				formatText(fmt.Sprintf("%2d", datum), statuses[i], &s)
			}
		}
	} else {
		s.Runes = append(s.Runes, ' ')
		s.FG = append(s.FG, termbox.ColorDefault)
		s.BG = append(s.BG, termbox.ColorDefault)
	}
	rowToHeader(&s, d.Name, hServices)
}

func formatText(text string, status int, s *styledText) {
	for i, r := range text {
		if silent && status == statusOK {
			if i == len(text)-1 {
				s.Runes = append(s.Runes, smallCircle)
			} else {
				s.Runes = append(s.Runes, ' ')
			}
		} else {
			s.Runes = append(s.Runes, r)
		}
		if status == statusOK {
			if silent {
				s.FG = append(s.FG, 9)
			} else {
				s.FG = append(s.FG, 3)
			}
		} else if status == statusWarning {
			s.FG = append(s.FG, 4|termbox.AttrBold)
		} else {
			s.FG = append(s.FG, 2|termbox.AttrBold)
		}
		s.BG = append(s.BG, termbox.ColorDefault)
	}
}

func drawHeader() {
	currentTab := 1
	for _, h := range tic.Header {
		position := currentTab
		name := h
		if tic.ColumnAlignment[h] == alignCentre {
			position += ((getFromColumnWidthMap(h) - len(name)) / 2)
		} else if tic.ColumnAlignment[h] == alignRight {
			position += (getFromColumnWidthMap(h) - len(name))
		}
		for j, r := range name {
			termbox.SetCell(3+position+j, headerRow, r, 9|termbox.AttrBold, termbox.ColorDefault)
		}
		currentTab += getFromColumnWidthMap(h) + 1
	}
}

func drawDate() {
	w := 1
	for _, h := range tic.Header {
		w += getFromColumnWidthMap(h) + 1
	}
	d := fetchTime.Format(time.RFC1123)
	for j, r := range d {
		termbox.SetCell((w-len(d))/2+j, dateRow, r, 8, termbox.ColorDefault)
	}
}

func drawStatusBar() {
	w, h := termbox.Size()
	if search {
		for i, r := range fmt.Sprintf("search: %s", searchString) {
			termbox.SetCell(i+1, h-1, r, 2, termbox.ColorDefault)
		}
	}
	if showIPs {
		for i, r := range "[IP]" {
			termbox.SetCell(w-13+i, h-1, r, 2, termbox.ColorDefault)
		}
	}
	if silent {
		for i, r := range "[S]" {
			termbox.SetCell(w-8+i, h-1, r, 2, termbox.ColorDefault)
		}
	}
	if forceReConnect {
		for i, r := range "[F]" {
			termbox.SetCell(w-4+i, h-1, r, 2, termbox.ColorDefault)
		}
	}
}

func formatDuration(duration int64) string {
	h := int64(float64(duration / 3600.0))
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

func drawAtIndex(i int, name string, flush bool) {
	w, h := termbox.Size()
	if i < startPosition || i > startPosition+h-2-dataStartRow {
		return
	}
	row := i - startPosition + dataStartRow
	bg := termbox.ColorDefault
	indexFg := termbox.Attribute(9)
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
	index := fmt.Sprintf(indexFormat, i+1)
	for j, r := range index {
		termbox.SetCell(currentTab+j, row, r, indexFg, bg)
	}
	currentTab += len(index) + 1
	for heidx, he := range tic.Header {
		position := currentTab
		s := tic.Data[name][heidx]
		if tic.ColumnAlignment[he] == alignCentre {
			position += ((getFromColumnWidthMap(he) - len(s.Runes)) / 2)
		} else if tic.ColumnAlignment[he] == alignRight {
			position += (getFromColumnWidthMap(he) - len(s.Runes))
		}
		for j := 0; j < len(s.Runes); j++ {
			fg := s.FG[j]
			bg := s.BG[j]
			if selected {
				fg = termbox.ColorBlack | termbox.AttrBold
				if bg == termbox.ColorDefault {
					fg = selectedFg
					bg = selectedBg
				}
			}
			termbox.SetCell(position+j, row, s.Runes[j], fg, bg)
		}
		currentTab += getFromColumnWidthMap(he) + 1
	}

	errorLayerMutex.Lock()
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
			termbox.SetCell(len(index)+getFromColumnWidthMap(hMachine)+j+3, row, r, fg, bg)
		}
	}
	errorLayerMutex.Unlock()
	if flush {
		termbox.Flush()
	}
}

func adjustStartPosition() {
	_, h := termbox.Size()
	limit := len(tic.Data)
	if search {
		limit = matchingCount
	}
	if h > (limit + dataStartRow) {
		startPosition = 0
	} else {
		if startPosition+h-1-dataStartRow > limit {
			startPosition = limit - (h - dataStartRow)
		}
	}
}

func getSelectedMachine() *machine {
	key := sorter.keys[cursorPosition]
	if search {
		i := 0
		for _, k := range sorter.keys {
			if matchingMachines[k] {
				if i == cursorPosition {
					key = k
				}
				i++
			}
		}
	}
	return machines[key]
}

func openConsole(command string) {
	m := getSelectedMachine()
	name := m.Name
	user := m.config.User
	if len(strings.TrimSpace(command)) > 0 {
		command = fmt.Sprintf("%s; bash -l", command)
	}
	if len(*terminal) == 0 {
		*terminal = "urxvt"
	}
	params := []string{"-e", "ssh", "-t", fmt.Sprintf("%s@%s", user, name), "-p", m.Port, command}
	cmd := exec.Command(*terminal, params...)
	go func() {
		err := cmd.Run()
		if err != nil {
			fmt.Println(err.Error())
		}
	}()
}

func handleArrowUp() {
	if cursorPosition > 0 {
		if cursorPosition == startPosition {
			if startPosition > 0 {
				startPosition--
			}
		}
		cursorPosition--
		sendRedrawRequest()
	}
}

func handleArrowDown() {
	_, h := termbox.Size()
	limit := len(tic.Data)
	if search {
		limit = matchingCount
	}
	if cursorPosition < limit-1 {
		cursorPosition++
		if cursorPosition == startPosition+(h-1-dataStartRow) {
			if startPosition < limit-2 {
				startPosition++
			}
		}
		sendRedrawRequest()
	}
}

func handleKeyEnd() {
	_, h := termbox.Size()
	limit := len(tic.Data)
	if search {
		limit = matchingCount
	}
	cursorPosition = limit - 1
	if limit < h-1-dataStartRow {
		startPosition = 0
	} else {
		startPosition = limit - (h - 1 - dataStartRow)
	}
	sendRedrawRequest()
}

func handlePageDown() {
	_, h := termbox.Size()
	pageSize := h - 1 - dataStartRow
	dataLength := len(tic.Data)
	if search {
		dataLength = matchingCount
	}
	if cursorPosition+pageSize < dataLength {
		cursorPosition += pageSize
	} else {
		cursorPosition = dataLength - 1
	}
	if dataLength < pageSize {
		startPosition = 0
	} else if startPosition+pageSize < dataLength-pageSize {
		startPosition += pageSize
	} else {
		startPosition = dataLength - pageSize
	}
	sendRedrawRequest()
}

func handlePageUp() {
	_, h := termbox.Size()
	pageSize := h - 1 - dataStartRow
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
	sendRedrawRequest()
}

func handleBackspace() {
	if search {
		if len(searchString) > 0 {
			searchString = searchString[0 : len(searchString)-1]
		} else {
			search = false
			searchString = ""
		}
	}
	formatAll()
	sendRedrawRequest()
}

func handleCtrlA() {
	if !running {
		go func(forceReConnect bool) {
			fetchTime = time.Now()
			drawDate()
			runOnHosts(forceReConnect)
		}(forceReConnect)
	}
}

func handleCtrlR() {
	m := getSelectedMachine()
	if !m.Fetching {
		go func(forceReConnect bool) {
			fetchTime = time.Now()
			drawDate()
			runOnHost(m.Name, forceReConnect)
		}(forceReConnect)
	}
}

func handleKeyPressInSearch(r rune) {
	if r > 31 && r < 127 && len(searchString) < 50 {
		searchString += string(r)
		cursorPosition = 0
		matchingCount = 0
		for k := range matchingMachines {
			matchingMachines[k] = false
		}
		formatAll()
		sendRedrawRequest()
	}
}

func getFromColumnWidthMap(key string) int {
	columnWidthMutex.Lock()
	val := tic.ColumnWidth[key]
	columnWidthMutex.Unlock()
	return val
}

func putToColumnWidthMap(key string, value int) {
	columnWidthMutex.Lock()
	tic.ColumnWidth[key] = value
	columnWidthMutex.Unlock()
}

func keyLoop() {
loop:
	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyF1:
				openConsole(f1)
			case termbox.KeyF2:
				openConsole(f2)
			case termbox.KeyF3:
				openConsole(f3)
			case termbox.KeyF4:
				openConsole(f4)
			case termbox.KeyF5:
				openConsole(f5)
			case termbox.KeyF6:
				openConsole(f6)
			case termbox.KeyF7:
				openConsole(f7)
			case termbox.KeyF8:
				openConsole(f8)
			case termbox.KeyF9:
				openConsole(f9)
			case termbox.KeyF10:
				openConsole(f10)
			case termbox.KeyF11:
				openConsole(f11)
			case termbox.KeyF12:
				openConsole(f12)
			case termbox.KeyCtrlA:
				handleCtrlA()
			case termbox.KeyCtrlR:
				handleCtrlR()
			case termbox.KeyCtrlF:
				search = true
				sendRedrawRequest()
			case termbox.KeyArrowUp:
				handleArrowUp()
			case termbox.KeyArrowDown:
				handleArrowDown()
			case termbox.KeyEnter:
				openConsole("")
			case termbox.KeyEnd:
				handleKeyEnd()
			case termbox.KeyHome:
				cursorPosition = 0
				startPosition = 0
				sendRedrawRequest()
			case termbox.KeyPgdn:
				handlePageDown()
			case termbox.KeyPgup:
				handlePageUp()
			case termbox.KeyBackspace2:
				handleBackspace()
			case termbox.KeyEsc:
				if search {
					search = false
					searchString = ""
					matchingCount = 0
					formatAll()
					sendRedrawRequest()
				} else {
					break loop
				}
			}
			if search {
				handleKeyPressInSearch(ev.Ch)
			} else {
				switch ev.Ch {
				case 102: // f
					forceReConnect = !forceReConnect
				case 115: // s - silent
					silent = !silent
					for _, h := range tic.Header {
						l := len(h)
						putToColumnWidthMap(h, l)
					}
					formatAll()
				case 105: // i - show IP-s
					showIPs = !showIPs
					for _, h := range tic.Header {
						l := len(h)
						putToColumnWidthMap(h, l)
					}
					formatAll()
				}
				sendRedrawRequest()
			}
		case termbox.EventResize:
			sendRedrawRequest()
		}
	}
}

func sendRedrawRequest() {
	redrawRequestChannel <- true
}

func redrawRoutine() {
	for {
		<-redrawRequestChannel
		for len(redrawRequestChannel) > 0 {
			<-redrawRequestChannel
		}
		redrawMutex.Lock()
		redraw()
		redrawMutex.Unlock()
	}
}

func runCli() {
	err := termbox.Init()
	if err != nil {
		panic(err)
	}
	defer termbox.Close()
	termbox.SetOutputMode(termbox.Output256)
	sendRedrawRequest()
	keyLoop()
}
