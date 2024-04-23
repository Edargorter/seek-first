package main

import (
    "encoding/xml"
	"encoding/csv"
    "fmt"
	"io"
	"strings"
    "os"
	"os/exec"
	"log"
	// "syscall"
	"sync"
	"time"
	"golang.org/x/term"
)

const (
	project_name 	= "Quick-Search"
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

var (
	os_cmds = make(map[string]string)
	win_width = 75
	win_height = 200
	debug_mode = false
	old_state *term.State
	lock sync.Mutex
	bible Bible 
	bks []BookName
	// listing []string
	inp string
	exitSIG = make(chan struct{})
	esc = map[string]string{"reset": "\u001b[0m",
		"bg_yellow":  "\u001b[43m",
		"bg_blue":    "\u001b[44m",
		"bg_white":   "\u001b[47;1m",
		"green":      "\u001b[32m",
		"black":      "\u001b[30m",
		"red":        "\u001b[31m",
		"backspace":  "\b\033[K",
		"cursorleft": "\x1b[1D"}
)

//Clear screen
func cls() {
	cmd := exec.Command(os_cmds["clear"])
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
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
										j + 1,
										k + 1,
										esc["reset"])
					dispverse := fmt.Sprintf("%s%s%s%s%s",
											verse[:index],
											esc["green"],
											verse[index:index + kp_len],
											esc["reset"],
											verse[index + kp_len:])
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
		default:
			inp += string(buf[:])
		/*
		case 0x15:
			fmt.Print(get_n_string(esc["backspace"], len(cmd_str)))
			cmd_str = ""
			*/
		}
		lock.Unlock()
	}
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
	for _, record := range records {
		if len(record) >= 2 {
			bk := BookName{Name: record[0], Abbr: record[1]}
			bks = append(bks, bk)
		}
	}

	// Test search 
	/*
	listing := search("predestined")
	for _, result := range listing {
		fmt.Println(result)
	}
	*/
	go updateListing()
	go handleSearch()

	for {
		lock.Lock()
		time.Sleep(1 * time.Second)
		lock.Unlock()
	}

}
