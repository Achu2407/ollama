package readline

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/emirpasic/gods/v2/lists/arraylist"
)

type History struct {
	Buf      *arraylist.List[string]
	Autosave bool
	Pos      int
	Limit    int
	Filename string
	Enabled  bool
}

func NewHistory() (*History, error) {
	fmt.Println("Creating new history instance")
	h := &History{
		Buf:      arraylist.New[string](),
		Limit:    100, // resizeme
		Autosave: true,
		Enabled:  true,
	}

	err := h.Init()
	if err != nil {
		fmt.Printf("Error initializing history: %v\n", err)
		return nil, err
	}

	fmt.Printf("History initialized successfully. Current size: %d\n", h.Size())
	return h, nil
}

func (h *History) Init() error {
	fmt.Println("Initializing history")
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting user home directory: %v\n", err)
		return err
	}

	path := filepath.Join(home, ".ollama", "history")
	fmt.Printf("History file path: %s\n", path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Printf("Error creating history directory: %v\n", err)
		return err
	}

	h.Filename = path

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("History file doesn't exist yet - will create new one")
			return nil
		}
		fmt.Printf("Error opening history file: %v\n", err)
		return err
	}
	defer f.Close()

	fmt.Println("Reading existing history file")
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Println("Finished reading history file")
				break
			}
			fmt.Printf("Error reading history file: %v\n", err)
			return err
		}

		line = strings.TrimSpace(line)
		if len(line) == 0 {
			fmt.Println("Skipping empty line in history")
			continue
		}

		fmt.Printf("Adding line to history: %s\n", line)
		h.Add(line)
	}

	return nil
}

func (h *History) Add(s string) {
	fmt.Printf("Adding new entry to history: %s\n", s)
	h.Buf.Add(s)
	h.Compact()
	h.Pos = h.Size()
	if h.Autosave {
		fmt.Println("Autosave enabled - saving history")
		_ = h.Save()
	}
}

func (h *History) Compact() {
	s := h.Buf.Size()
	if s > h.Limit {
		fmt.Printf("Compacting history - current size %d exceeds limit %d\n", s, h.Limit)
		for range s - h.Limit {
			h.Buf.Remove(0)
		}
		fmt.Printf("History compacted - new size: %d\n", h.Buf.Size())
	}
}

func (h *History) Clear() {
	fmt.Println("Clearing history")
	h.Buf.Clear()
}

func (h *History) Prev() (line string) {
	fmt.Printf("Getting previous history entry (current pos: %d)\n", h.Pos)
	if h.Pos > 0 {
		h.Pos -= 1
	}
	line, _ = h.Buf.Get(h.Pos)
	fmt.Printf("Returning history entry: %s (new pos: %d)\n", line, h.Pos)
	return line
}

func (h *History) Next() (line string) {
	fmt.Printf("Getting next history entry (current pos: %d)\n", h.Pos)
	if h.Pos < h.Buf.Size() {
		h.Pos += 1
		line, _ = h.Buf.Get(h.Pos)
		fmt.Printf("Returning history entry: %s (new pos: %d)\n", line, h.Pos)
	} else {
		fmt.Println("Already at newest history position")
	}
	return line
}

func (h *History) Size() int {
	size := h.Buf.Size()
	fmt.Printf("Getting history size: %d\n", size)
	return size
}

func (h *History) Save() error {
	if !h.Enabled {
		fmt.Println("History disabled - not saving")
		return nil
	}

	fmt.Println("Saving history to file")
	tmpFile := h.Filename + ".tmp"
	fmt.Printf("Using temp file: %s\n", tmpFile)

	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Printf("Error opening temp file for writing: %v\n", err)
		return err
	}
	defer f.Close()

	buf := bufio.NewWriter(f)
	for cnt := range h.Size() {
		line, _ := h.Buf.Get(cnt)
		fmt.Fprintln(buf, line)
	}
	buf.Flush()
	f.Close()

	fmt.Printf("Renaming temp file to %s\n", h.Filename)
	if err = os.Rename(tmpFile, h.Filename); err != nil {
		fmt.Printf("Error renaming temp file: %v\n", err)
		return err
	}

	fmt.Println("History saved successfully")
	return nil
}
