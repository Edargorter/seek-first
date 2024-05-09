package main

import (
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	// "bufio"
	// "syscall"
	"math"
	"sync"
	// "time"
	"golang.org/x/term"
)

const (
	project_name = "Seek-First"
	about_str     = "Made by Edargorter (Zachary D. Bowditch) 2024.\r\n -- May all be added unto you --"
	help_str      = "Search biblical address (e.g. '1 Peter 3:15, 4:11, Jeremiah 2')\r\n or search keyphrase (e.g. '!seek first')"
)

type Bible struct {
	Books []Book `xml:"b"`
}

type Book struct {
	Chapters []Chapter `xml:"c"`
}

type Chapter struct {
	Verses []string `xml:"v"`
}

type BookName struct {
	Name string
	Abbr string
}

type Address struct {
	Book    string
	Chapter int
	Start   int
	End     int
}

type SearchResult struct {
	Listing []string
	Stats []struct {
		Book string
		Occ int 
	}
}

var (
	update     = make(chan bool)
	inp_buf    = make([]byte, 1)
	// os_cmds    = make(map[string]string)
	win_width  = 75
	win_height = 200
	debug_mode = false
	old_state  *term.State
	lock       sync.Mutex
	path       = "data/"
	bible      Bible
	bks        []BookName
	// listing []string
	inp     string
	exitSIG = make(chan struct{})
	esc     = map[string]string{"reset": "\u001b[0m",
								"bg_yellow":  "\u001b[43m",
								"bg_blue":    "\u001b[44m",
								"bg_white":   "\u001b[47;1m",
								"green":      "\u001b[32m",
								"black":      "\u001b[30m",
								"red":        "\u001b[31m",
								"backspace":  "\b\033[K",
								"cursorleft": "\x1b[1D",
								"rightn"    :  "\033[%dC", // format string 
								"clear": "\033[2J",
								"topLeft": "\033[H"}
	lookup    = make(map[string]int)
	bookRegex = regexp.MustCompile(`(\d+\s)?([A-Za-z]+)`)
	chapRegex = regexp.MustCompile(`\d+`)
	bookChapRegex = regexp.MustCompile(` (\d+)`)
	chapVerseRegex = regexp.MustCompile(`(\d+):(\d+)(-\d+)?`)
	//chapVerseRangeRegex = regexp.MustCompile(`\d+\s+[A-Za-z]+\s+\d+:(\d+)-(\d+)?`)
)

// Helper functions 
func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

// Return string concatenated N times 
func getNString(s string, n int) string {
	nstr := ""
	for i := 0; i < n; i++ {
		nstr += s
	}
	return nstr
}

// Clear screen
func cls() {
	//fmt.Print(esc["clear"])
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}	
}

func getPassages(addr Address, listing *[]string) {
	bookIndex, found := lookup[procStr(addr.Book)]
	if found { // bookIndex < len(bible.Books) {
		book := bible.Books[bookIndex]
		nChapters := len(book.Chapters) - 1
		// Now, we avoid "chapter -1" and N > no. of chapters in book 
		chapterIndex := max(min(addr.Chapter, nChapters), 0) 
		chapter := book.Chapters[chapterIndex]
		if addr.Start == -1 && addr.End == -1 {
			// This means the whole chapter
			addr.Start = 0
			addr.End = len(chapter.Verses) - 1
		} else if addr.End == -1 {
			// Just one first
			addr.End = addr.Start
		}
		if lc := len(chapter.Verses) - 1; addr.End > lc {
			addr.End = lc
		}
		ref := fmt.Sprintf("%s%s%s %d:",
			esc["bg_white"],
			esc["black"],
			bks[bookIndex].Abbr,
			chapterIndex+1)

		// Now generate range
		for i := addr.Start; i <= addr.End; i++ {
			dispverse := fmt.Sprintf("%s%d%s %s",
				ref,
				i+1,
				esc["reset"],
				chapter.Verses[i])
			(*listing) = append((*listing), dispverse)
		}
	}
}

func processToken(ref string, prevBook *string, listing *[]string) (string, error) {
	msg := ""	
	ref = procStr(ref)

	// for quick reference 
	if ref == "about" {
		msg = about_str
		return msg, nil
	} else if ref == "help" {
		msg = help_str
		return msg, nil
	}
	bookref := bookRegex.FindAllString(ref, -1)
	//chapverrange := chapVerseRangeRegex.FindAllString(ref, -1)
	chapver := chapVerseRegex.FindAllString(ref, -1)
	chap := bookChapRegex.FindAllString(ref, -1)
	book := ""

	if len(bookref) == 0 {
		// Book not found
		if *prevBook == "" {
			msg = fmt.Sprintf("%s (book not found)", ref)
			return msg, nil
		} else {
			book = *prevBook
			chap = chapRegex.FindAllString(ref, -1)
		}
	} else {
		book = bookref[0]
	}

	if len(chap) == 0 {
		// no chapter found
		msg = fmt.Sprintf("%s (chapter not found)", ref)
		return msg, nil
	} 
	addr := Address{Book: book, Chapter: -1, Start: -1, End: -1}
	num, err := strconv.Atoi(strings.TrimSpace(chap[0]))
	if err != nil {
		msg = fmt.Sprintf("Cannot convert chapter %s to int", chap[0])
		return msg, err
	}
	addr.Chapter = num - 1

	if len(chapver) > 0 {
		cv := strings.Split(chapver[0], ":")
		// Add chapter
		verses := strings.Split(cv[1], "-")
		num, err = strconv.Atoi(verses[0])
		if err != nil {
			// Shouldn't get here because of regex
			msg = fmt.Sprintf("Cannot convert %s to int", verses[0])
			return msg, err 
		}
		addr.Start = num - 1
		if len(verses) > 1 {
			num, err = strconv.Atoi(verses[1])
			if err != nil {
				// Shouldn't get here because of regex
				msg = fmt.Sprintf("Cannot convert %s to int", verses[1])
				return msg, err 
			}
			addr.End = num - 1
		}
	}
	getPassages(addr, listing)
	*prevBook = book
	return msg, nil
}

func getTexts(searchstr string) []string {
	var listing []string
	refs := strings.Split(searchstr, ",")
	prevBook := ""
	for _, ref := range refs {
		msg, err := processToken(ref, &prevBook, &listing)
		if err == nil {
			if msg != "" {
				listing = append(listing, msg)
			}
		}
	}
	return listing
}

func search(keyphrase string, stats *[]int) []string {
	var dispstr = ""
	var listing []string
	kp_len := len(keyphrase)
	keyphrase = strings.ToLower(keyphrase)
	for i := range bks {
		book := bible.Books[i]
		(*stats)[i] = 0
		for j := range book.Chapters {
			chapter := book.Chapters[j]
			for k := range chapter.Verses {
				verse := chapter.Verses[k]
				index := strings.Index(strings.ToLower(verse),
					keyphrase)
				if index != -1 {
					(*stats)[i] += 1
					ref := fmt.Sprintf("%s%s%s %d:%d%s",
						esc["bg_white"],
						esc["black"],
						bks[i].Abbr,
						j+1,
						k+1,
						esc["reset"])
					dispverse := fmt.Sprintf("%s%s%s%s%s",
						verse[:index],
						esc["green"],
						verse[index:index+kp_len],
						esc["reset"],
						verse[index+kp_len:])
					// need to format verse with highlighted substring
					dispstr = fmt.Sprintf("%s %s",
						ref,
						dispverse)
					listing = append(listing, dispstr)
					// fmt.Println(dispstr)
				}
			}
		}
	}
	return listing
}

func updateListing() {
	stats := make([]int, len(bks))
	header := "Seek-First"
	cls()
	fmt.Printf("%s> ", header)
	prev := ""
	width := float64(win_width)
	for {
		select {
		case <- exitSIG:
			return
		case <- update:
			// lock.Lock()
			var listing []string
			if inp == prev {
				continue
			}
			prev = inp
			cls()
			fmt.Printf("%s> %s\r\n", header, inp)
			// fmt.Print("Search> ", inp, "\r\n")
			count := 0
			if len(inp) > 2 {
				if inp[0] != '!' {
					listing = getTexts(procStr(inp))
				} else {
					listing = search(procStr(inp[1:]), &stats)
					total := 0
					for i := 0; i < len(stats); i++ {
						if stats[i] > 0 {
							fmt.Print(bks[i].Abbr, " (", stats[i], ") ")
							total += stats[i]
						}
					}
					if total > 0 {
						fmt.Print("Total: ", total)
					}
					fmt.Print("\r\n")
				}
			}
			for _, result := range listing {
				//lines := int(math.Ceil(len(result) / win_width))
				lines := math.Ceil(float64(len(result)) / width)
				count += int(lines)
				if count >= win_height + 15 {
					break
				}
				fmt.Print(result + "\r\n")
			}
			// lock.Unlock()
			fmt.Print(esc["topLeft"], fmt.Sprintf(esc["rightn"], len(header) + 2 + len(inp)))
			//time.Sleep(10 * time.Millisecond)
		}
	}
}

func safeQuit(args chan struct{}) bool {
	cls()
	term.Restore(int(os.Stdin.Fd()), old_state)
	os.Exit(0)
	return true
}

func handleSearch() {
	// fmt.Print("Search\r\n")
	for {
		/*
		reader := bufio.NewReader(os.Stdin)
		_, err := reader.ReadByte()
		*/

		// Read a single byte 
		_, err := os.Stdin.Read(inp_buf)
		//fmt.Print("read\r\n")
		if err != nil {
			safeQuit(exitSIG)
		}
		update <- true 
		c := inp_buf[0]
		// lock.Lock()
		switch c {
			case 0x3:
				safeQuit(exitSIG)
				// lock.Unlock()
				return

			// Backspace, Ctrl-h
			case 0x08, 0x7f:
				if len(inp) > 0 {
					inp = inp[:len(inp)-1]
				}

			// Ctrl-u
			case 0x15:
				inp = ""

			// Ctrl-w (remove single word)
			case 0x17:
				index := strings.LastIndexByte(strings.TrimRight(inp, " "), ' ')
				if index < len(inp) {
					index++
				}
				inp = inp[:index]

			// White space 
			case 0x20:
				inp += " "

			default:
				// fmt.Print("Char: ", c)
				inp += string(c)
		}
		// lock.Unlock()
	}
}

func procStr(str string) string {
	re := regexp.MustCompile(`\s+`)
	str = strings.ToLower(re.ReplaceAllString(strings.TrimSpace(str), " "))
	return str
}

func main() {

	//Get terminal dimensions
	if term.IsTerminal(0) {
		width, height, err := term.GetSize(0)
		if err != nil {
			log.Printf("Using default width %v\n", win_width)
		} else {
			win_width = width
			win_height = height
		}
	}

	//Terminal Raw Mode if not in debug mode
	if !debug_mode {
		prev_state, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			log.Fatalf(err.Error())
		}
		old_state = prev_state
		//Switch back to old state
	}

	filename := path + "esv.xml"
	xmlFile, err := os.Open(filename)
	if err != nil {
		fmt.Printf("Error opening XML file:\r\n", err)
		return
	}

	defer xmlFile.Close()

	xmlData, err := io.ReadAll(xmlFile)
	if err != nil {
		fmt.Printf("Error reading from XML file\r\n")
		return
	}

	err = xml.Unmarshal(xmlData, &bible)
	if err != nil {
		fmt.Printf("Error unmarshalling XML: %v\r\n", err)
		return
	}

	// Printing the parsed data
	for _, b := range bible.Books {
		fmt.Sprintf("%s (%d) chapters", b, len(b.Chapters))
	}

	// Read book names
	filename = path + "bible-books.csv"
	file, err := os.Open(filename)
	if err != nil {
		fmt.Print("Error opening csv:\r\n", err)
		return
	}
	defer file.Close()
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Print("Error reading csv:\r\n", err)
		return
	}
	for i, record := range records {
		if len(record) >= 2 {
			bk := BookName{Name: record[0], Abbr: record[1]}
			bks = append(bks, bk)
			lookup[procStr(record[0])] = i
		}
	}
	go updateListing()
	handleSearch()
}
