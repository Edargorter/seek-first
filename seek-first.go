package main

import (
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	// "os/exec"
	"regexp"
	"strconv"
	"strings"
	// "bufio"
	// "syscall"
	// "math"
	"sync"
	"sort"
	//"slices"
	//"time"
	"golang.org/x/term"
)

const (
	project_name = "Seek-First"
	about_str     = "Made by Edargorter (Zachary D. Bowditch) 2024.\r\n -- May \"all these things\" be added unto you --"
	help_str      = "Search biblical address (e.g. '1 Peter 3:15, 4:11, Jeremiah 2')\r\n" + 
	"or search keyphrase (e.g. '!seek first')\r\n" +
	"Commands:\r\n" +
	"\t- help (displays this message)\r\n" +
	"\t- quit (exits session)\r\n" + 
	"\t- about (author and purpose)\r\n" +
	"\t- Ctrl-w (delete word)\r\n" +
	"\t- Ctrl-u (delete entire input)\r\n"
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

func (addr Address) isSame(other *Address) bool {
	return addr.Book == (*other).Book &&
			addr.Chapter == (*other).Chapter &&
			addr.Start == (*other).Start &&
			addr.End == (*other).End 
}

func (addr *Address) setAddress(other *Address) {
	addr.Book = (*other).Book
	addr.Chapter = (*other).Chapter 
	addr.Start = (*other).Start 
	addr.End = (*other).End 
}

type SearchResult struct {
	Listing []string
	Stats []struct {
		Book string
		Occ int 
	}
}

type TDims struct {
	width int
	height int 
}

var (
	update     = make(chan bool)
	inp_buf    = make([]byte, 1)
	stats 	   []int
	bk_indices []int
	num_bg_colours int
	bg_colours []string
	bg_index  	int
	// os_cmds    = make(map[string]string)
	win_width  = 75
	win_height = 200
	debug_mode = false
	old_state  *term.State
	lock       sync.Mutex
	path       = "data/"
	bible      Bible
	bks        []BookName
	inp        string
	tabpressed = false
	exitSIG = make(chan struct{})
	esc     = map[string]string{"reset": "\u001b[0m",
								"bg_yellow":  "\u001b[43m",
								"bg_blue":    "\u001b[44m",
								"bg_white":   "\u001b[47;1m",
								"green":      "\u001b[32m",
								"black":      "\u001b[30m",
								"bg_light_orange":"\033[48;5;215m",
								"bg_light_red":	"\033[48;5;203m",
								"red":        "\u001b[31m",
								"grey":	      "\u001b[90m",
								"backspace":  "\b\033[K",
								"cursorleft": "\x1b[1D",
								"rightn"    :  "\033[%dC", // format string (n)
								//"clear": "\033[2J",
								"clear": "\033c",
								"toPos": "\033[%d;%dH", // format string (row, col)
								"bottomLeft": "\033[%d;1H", //format string (row)
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

func getTerminalDims(t_dims *TDims) {
	//Get terminal dimensions
	if term.IsTerminal(0) {
		width, height, err := term.GetSize(0)
		if err != nil {
			log.Printf("Using default width %v\n", win_width)
		} else {
			(*t_dims).height = height
			(*t_dims).width = width
		}
	}
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
	fmt.Print(esc["clear"])
	/*
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}	
	*/
}

func getPassages(addr Address, listing *[]string) bool {
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
			esc[bg_colours[bg_index]],
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
	if addr.Start > -1 && addr.End >= addr.Start {
		// Then, we've added to listing 
		return true
	}
	return false 
}

func getBibleReference(ref string, prevAddr *Address, listing *[]string) bool {
	bookref := bookRegex.FindAllString(ref, -1)
	//chapverrange := chapVerseRangeRegex.FindAllString(ref, -1)
	chapver := chapVerseRegex.FindAllString(ref, -1)
	chap := bookChapRegex.FindAllString(ref, -1)
	book := ""

	if len(bookref) == 0 {
		// Book not found
		if (*prevAddr).Book == "" {
			(*listing) = append((*listing), fmt.Sprintf("%s (book not found)", ref))
			return false 
		} else {
			book = (*prevAddr).Book
			chap = chapRegex.FindAllString(ref, -1)
		}
	} else {
		book = bookref[0]
	}

	if len(chap) == 0 {
		// no chapter found
		(*listing) = append((*listing), fmt.Sprintf("%s (chapter not found)", ref))
		return false 
	} 

	chapter, err := strconv.Atoi(strings.TrimSpace(chap[0]))
	if err != nil {
		(*listing) = append((*listing), fmt.Sprintf("Cannot convert chapter %s to int", chap[0]))
		return false 
	} 
	
	addr := Address{Book: book, Chapter: -1, Start: -1, End: -1}
	addr.Chapter = chapter - 1

	if len(chapver) > 0 {
		// We have verses 
		cv := strings.Split(chapver[0], ":")
		// Add chapter
		verses := strings.Split(cv[1], "-")
		startverse, err := strconv.Atoi(verses[0])
		if err != nil {
			// Shouldn't get here because of regex
			(*listing) = append((*listing), fmt.Sprintf("Cannot convert %s to int", verses[0]))
			return false 
		}
		addr.Start = startverse - 1
		if len(verses) > 1 {
			endverse, err := strconv.Atoi(verses[1])
			if err != nil {
				// Shouldn't get here because of regex
				(*listing) = append((*listing), fmt.Sprintf("Cannot convert %s to int", verses[1]))
				return false
			}
			addr.End = endverse - 1
		}
	}
	changeBgIndex := false
	if !addr.isSame(prevAddr) {
		// If we've not just seen the same address
		changeBgIndex = getPassages(addr, listing) 
	}
	prevAddr.setAddress(&addr)
	return changeBgIndex
}

func getSearchResult(searchstr string, listing *[]string) bool {

	var dispstr = ""
	kp_len := len(searchstr)

	// Find matching verses and track stats 
	for i := range bks {
		book := bible.Books[i]
		stats[i] = 0
		for j := range book.Chapters {
			chapter := book.Chapters[j]
			for k := range chapter.Verses {
				verse := chapter.Verses[k]
				index := strings.Index(strings.ToLower(verse),
					searchstr)
				if index != -1 {
					stats[i] += 1
					ref := fmt.Sprintf("%s%s%s %d:%d%s",
						esc[bg_colours[bg_index]],
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
					(*listing) = append((*listing), dispstr)
					// fmt.Println(dispstr)
				}
			}
		}
	}

	// Sort stats in descending order 
	sort.Slice(bk_indices, func(i, j int) bool {
		return stats[bk_indices[i]] > stats[bk_indices[j]]
	})

	// Add to display listing 
	total := 0
	statsstr := fmt.Sprintf("\"%s\" : ", searchstr)
	for i := 0; i < len(bk_indices); i++ {
		curr := stats[bk_indices[i]]
		if curr == 0 {
			break
		}
		total += curr
		statsstr += fmt.Sprintf(" %s [%d] ", bks[bk_indices[i]].Abbr, curr)
	}

	statsstr += fmt.Sprintf("Total: [%d]", total)
	(*listing) = append((*listing), statsstr)
	return true 
}

func processTokens(tokens []string, listing *[]string) {
	prevAddr := Address{Book: "", Chapter: -1, Start: -1, End: -1}
	prevToken := ""
	for _, token := range tokens {
		token = procStr(token)
		if len(token) == 0 || token == prevToken {
			continue 
		}
		prevToken = token
		// for quick reference 
		if token == "quit" {
			safeQuit(exitSIG)
		} else if token == "about" {
			(*listing) = append((*listing), about_str)
		} else if token == "help" {
			(*listing) = append((*listing), help_str)
		} else if len(token) > 3 && token[0] == '!'{
			// Then we search for a string in the text 
			if getSearchResult(token[1:], listing) {
				bg_index = (bg_index + 1) % num_bg_colours
			}
		} else {
			// Search Bible address 
			if getBibleReference(token, &prevAddr, listing) {
				bg_index = (bg_index + 1) % num_bg_colours
			}
		}
	}
}

func getTokens(inputstr string) []string {
	tokens := strings.Split(inputstr, ",")
	return tokens 
}

func getSuggestedSuffix(str string) string {
	str = procStr(str)
	if str == "" {
		return "help"
	}
	for i := range bks {
		book := bks[i].Name
		if len(book) > len(str) &&
		   procStr(book[:len(str)]) == str {
			// We have a match
			return book[len(str):]
		}
	}
	return ""
}

func updateListing() {

	// Initial terminal dimensions 
	t_dims := TDims{}
	t_dims.width = 0
	t_dims.height = 0
	getTerminalDims(&t_dims)

	// For match stats purposes 
	bk_indices = make([]int, len(bks))
	for i := 0; i < len(bks); i++ {
		bk_indices[i] = i
	}

	//Background colours for addresses
	bg_colours = []string{"bg_light_orange", "bg_light_red"}
	num_bg_colours = len(bg_colours)

	stats = make([]int, len(bks))
	prev := ""
	// width := float64(win_width)

	sugsuf := getSuggestedSuffix(inp)
	withsug := fmt.Sprintf("%s%s%s%s", inp, esc["grey"], sugsuf, esc["reset"])
	header := fmt.Sprintf("%s%s%s>%s", esc["bg_white"], esc["black"], project_name, esc["reset"])

	cls()
	//fmt.Printf("%s %s\r\n", header, withsug)
	//fmt.Print(esc["topLeft"], fmt.Sprintf(esc["rightn"], len(project_name) + 2 + len(inp)))

	// Move cursor to bottom of window and produce search prompt 
	fmt.Printf("%s%s %s%s", 
				fmt.Sprintf(esc["bottomLeft"], t_dims.height),
				header, 
				withsug, 
				fmt.Sprintf(esc["toPos"], t_dims.height, len(project_name) + 3 + len(inp)))

	for {
		select {
		case <- exitSIG:
			return
		case <- update:
			// lock.Lock()
			if tabpressed {
				tabpressed = false 
				inp = procStr(inp) + sugsuf
			}
			if inp == prev {
				continue
			}
			// Update terminal dimensions 
			getTerminalDims(&t_dims)

			// Display string processing 
			bg_index = 0
			prev = inp
			tokens := getTokens(inp)
			if inp == "" {
				sugsuf = "help"
			} else {
				sugsuf = getSuggestedSuffix(tokens[len(tokens)-1])
			}
			withsug := fmt.Sprintf("%s%s%s%s", inp, esc["grey"], sugsuf, esc["reset"])
			cls()

			// Find search results, if any 
			var listing []string
			processTokens(tokens, &listing)

			for _, result := range listing {
				fmt.Print(result)
				fmt.Print("\r\n")
			}
			//fmt.Print(esc["topLeft"], fmt.Sprintf(esc["rightn"], len(project_name) + 2 + len(inp)))
			//fmt.Print("\r\n")
			
			// Move cursor to bottom of window and produce search prompt 
			fmt.Printf("%s%s %s%s", 
						fmt.Sprintf(esc["bottomLeft"], t_dims.height),
						header, 
						withsug, 
						fmt.Sprintf(esc["toPos"], t_dims.height, len(project_name) + 3 + len(inp)))

			//fmt.Print(fmt.Sprintf(esc["rightn"], len(project_name) + 2 + len(inp)))

			// --------------------
			/*
			count := 0
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
			*/

			// Return cursor to end of input 
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
	for {
		// Read a single byte 
		_, err := os.Stdin.Read(inp_buf)
		if err != nil {
			safeQuit(exitSIG)
		}
		update <- true 
		c := inp_buf[0]
		// lock.Lock()
		switch {
			case c == 0x3:
				safeQuit(exitSIG)
				// lock.Unlock()
				return

			// Backspace, Ctrl-h
			case c == 0x08 || c == 0x7f:
				if len(inp) > 0 {
					inp = inp[:len(inp)-1]
				}

			// Ctrl-u
			case c == 0x15:
				inp = ""

			// Ctrl-w (remove single word)
			case c == 0x17:
				spaceindex := strings.LastIndexByte(strings.TrimRight(inp, " "), ' ')
				commaindex := strings.LastIndexByte(strings.TrimRight(inp, ","), ',')
				index := max(spaceindex, commaindex)
				if index < len(inp) {
					index++
				}
				inp = inp[:index]

			// White space 
			case c == 0x20:
				inp += " "
				
			//alpha-numeric and special chars 
			case c >= 0x21 && c <= 0x7A:
				inp += string(c)

			// Tab key for word completion
			case c == 0x09:
				tabpressed = true	
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
