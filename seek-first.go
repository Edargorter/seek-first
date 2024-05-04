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
	"time"
	"golang.org/x/term"
)

const (
	project_name = "Seek-First"
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
	bookIndex := lookup[procStr(addr.Book)]
	if bookIndex < len(bible.Books) {
		book := bible.Books[bookIndex]
		chapterIndex := addr.Chapter
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

func getTexts(searchstr string) []string {
	var listing []string
	refs := strings.Split(searchstr, ",")
	prevBook := ""
	for _, ref := range refs {
		ref = procStr(ref)

		bookref := bookRegex.FindAllString(ref, -1)
		//chapverrange := chapVerseRangeRegex.FindAllString(ref, -1)
		chapver := chapVerseRegex.FindAllString(ref, -1)
		chap := bookChapRegex.FindAllString(ref, -1)
		book := ""

		if len(bookref) == 0 {
			// Book not found
			if prevBook == "" {
				listing = append(listing, fmt.Sprintf("%s (book not found)", ref))
				break
			} else {
				book = prevBook
				chap = chapRegex.FindAllString(ref, -1)
			}
		} else {
			book = bookref[0]
		}
		if len(chap) == 0 {
			// no chapter found
			listing = append(listing, fmt.Sprintf("%s (chapter not found)", ref))
			break
		}


		addr := Address{Book: book, Chapter: -1, Start: -1, End: -1}
		num, err := strconv.Atoi(strings.TrimSpace(chap[0]))
		if err != nil {
			log.Fatal(fmt.Sprintf("Cannot convert chapter %s to int", chap[0]))
		}
		addr.Chapter = num - 1

		if len(chapver) > 0 {
			cv := strings.Split(chapver[0], ":")
			// Add chapter
			verses := strings.Split(cv[1], "-")
			num, err = strconv.Atoi(verses[0])
			if err != nil {
				log.Fatal(fmt.Sprintf("Cannot convert %s to int", verses[0]))
			}
			addr.Start = num - 1
			if len(verses) > 1 {
				num, err = strconv.Atoi(verses[1])
				if err != nil {
					log.Fatal(fmt.Sprintf("Cannot convert %s to int", verses[1]))
				}
				addr.End = num - 1
			}
		}

		getPassages(addr, &listing)
		prevBook = book
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

	cls()
	fmt.Print("Search> ")
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
			fmt.Print("Search> ", inp, "\r\n")
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
				if count >= win_height {
					break
				}
				fmt.Print(result + "\r\n")
			}
			// lock.Unlock()
			fmt.Print(esc["topLeft"], fmt.Sprintf(esc["rightn"], 8 + len(inp)))
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func safeQuit(args chan struct{}) bool {
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

			// White space 
			case 0x20:
				fmt.Print("White space")
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

	filename := "esv.xml"
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
	file, err := os.Open("bible-books.csv")
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
