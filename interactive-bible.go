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
	"bufio"
	// "syscall"
	"sync"
	// "time"
	"golang.org/x/term"
)

const (
	project_name = "Quick-Search"
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

var (
	os_cmds    = make(map[string]string)
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
		"cursorleft": "\x1b[1D"}
	lookup         = make(map[string]int)
	bookRegex      = regexp.MustCompile(`(\d+\s)?([A-Za-z]+)`)
	chapRegex      = regexp.MustCompile(` (\d+)`)
	chapVerseRegex = regexp.MustCompile(`(\d+):(\d+)(-\d+)?`)
	//chapVerseRangeRegex = regexp.MustCompile(`\d+\s+[A-Za-z]+\s+\d+:(\d+)-(\d+)?`)
)

// Clear screen
func cls() {
	cmd := exec.Command(os_cmds["clear"])
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
			dispverse := fmt.Sprintf("%s%d%s \"%s\"",
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
		chap := chapRegex.FindAllString(ref, -1)
		book := ""

		if len(bookref) == 0 {
			// Book not found
			if prevBook == "" {
				listing = append(listing, fmt.Sprintf("%s (book not found)", ref))
				break
			} else {
				book = prevBook
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

		if len(chapver) > 0 {
			cv := strings.Split(chapver[0], ":")
			// Add chapter
			num, err := strconv.Atoi(cv[0])
			if err != nil {
				log.Fatal(fmt.Sprintf("Cannot convert %s to int", cv[0]))
			}
			addr.Chapter = num - 1
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

func search(keyphrase string) []string {
	var dispstr = ""
	var listing []string
	kp_len := len(keyphrase)
	keyphrase = strings.ToLower(keyphrase)
	for i := range bks {
		book := bible.Books[i]
		for j := range book.Chapters {
			chapter := book.Chapters[j]
			for k := range chapter.Verses {
				verse := chapter.Verses[k]
				index := strings.Index(strings.ToLower(verse),
					keyphrase)
				if index != -1 {
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
					dispstr = fmt.Sprintf("%s \"%s\"",
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

/*

func updateListing() {
	fmt.Println("update")
	for {
		select {
		case <- exitSIG:
			return
		default:
			lock.Lock()
			var listing []string
			if inp != "" {
				listing = search(inp)
			}
			// search(inp)
			cls()
			fmt.Println(inp)
			for _, result := range listing {
				fmt.Println(result)
			}
			lock.Unlock()
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func handleSearch() {
	fmt.Println("search")
	var buf [1]byte
	for {
		os.Stdin.Read(buf[:])
		lock.Lock()
		switch buf[0] {

		case 0x3:
			close(exitSIG)
			lock.Unlock()
			return
		case 0x08, 0x7f:
			inp += string(esc["backspace"])
		case 0x15:
			fmt.Print(get_n_string(esc["backspace"], len(cmd_str)))
			cmd_str = ""
		default:
			inp += string(buf[:])
		}
		lock.Unlock()
	}
}
*/

func procStr(bookname string) string {
	re := regexp.MustCompile(`\s+`)
	procname := strings.ToLower(re.ReplaceAllString(strings.TrimSpace(bookname), " "))
	return procname
}

func main() {
	os_cmds["clear"] = "clear"

	//Get terminal dimensions
	if term.IsTerminal(0) {
		width, height, err := term.GetSize(0)
		if err != nil {
			//log.Printf("Using default width %v\n", win_width)
		} else {
			win_width = width
			win_height = height
		}
	}

	/*
		//Terminal Raw Mode if not in debug mode
		if !debug_mode {
			prev_state, err := term.MakeRaw(int(os.Stdin.Fd()))
			if err != nil {
				log.Fatalf(err.Error())
			}
			old_state = prev_state
			//Switch back to old state
		}
	*/

	filename := "esv.xml"
	xmlFile, err := os.Open(filename)
	if err != nil {
		fmt.Println("Error opening XML file:", err)
		return
	}

	defer xmlFile.Close()

	xmlData, err := io.ReadAll(xmlFile)
	if err != nil {
		fmt.Printf("Error reading from XML file")
		return
	}

	err = xml.Unmarshal(xmlData, &bible)
	if err != nil {
		fmt.Printf("Error unmarshalling XML: %v", err)
		return
	}

	// Printing the parsed data
	for _, b := range bible.Books {
		fmt.Sprintf("%s (%d) chapters", b, len(b.Chapters))
	}

	// Read book names
	file, err := os.Open("bible-books.csv")
	if err != nil {
		fmt.Println("Error opening csv:", err)
		return
	}
	defer file.Close()
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println("Error reading csv:", err)
		return
	}
	for i, record := range records {
		if len(record) >= 2 {
			bk := BookName{Name: record[0], Abbr: record[1]}
			bks = append(bks, bk)
			lookup[procStr(record[0])] = i
		}
	}
	fmt.Println("Search:")

	// Test search
	for {
		fmt.Print("> ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			input := scanner.Text()
			//input := "2 Peter 3:1-4"
			cls()
			fmt.Println(input)
			if input == "" {
				continue
			}
			var listing []string
			if input[0] != '!' {
				input = procStr(input)
				listing = getTexts(input)
			} else {
				input = procStr(input[1:])
				listing = search(input)
				fmt.Println(">", input)
			}
			count := 0
			for _, result := range listing {
				fmt.Println(result)
				if count == win_height {
					break
				}
			}
		}
		os.Stdout.Sync()
	}
	/*
		go updateListing()
		go handleSearch()

		for {
			lock.Lock()
			time.Sleep(1 * time.Second)
			lock.Unlock()
		}
	*/
}
