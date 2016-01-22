package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
)

var (
	filePattern = flag.String("pattern", "", "Pattern to glob with")
	socketName  = flag.String("name", "", "Name of socket file to create")
	listener    net.Listener
	listenerMut = &sync.Mutex{}
)

func catchSigint() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			listenerMut.Lock()
			if listener != nil {
				listener.Close()
			}
			log.Fatalf("Closing upon request: %q\n", sig)
			listenerMut.Unlock()
		}
	}()
}

func validateOptions() {
	if *socketName == "" {
		fmt.Println("You must provide a name")
		os.Exit(1)
	}
	if *filePattern == "" {
		fmt.Println("You must provide a pattern")
		os.Exit(1)
	}
}

func main() {
	flag.Parse()
	validateOptions()
	var err error
	catchSigint()
	listenerMut.Lock()
	listener, err = net.Listen("unix", *socketName)
	if err != nil {
		log.Fatalln(err)
	}
	defer listener.Close()
	listenerMut.Unlock()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatalln(err)
		}

		go feedFile(conn)
	}
}

func feedFile(conn net.Conn) {
	defer conn.Close()
	contents := make(chan io.Reader)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go serveContents(wg, conn, contents)

	files, err := filepath.Glob(*filePattern)
	if err != nil {
		log.Fatalf("Bad file globbing pattern: %q, %q\n", filePattern, err)
	}

	for _, fileName := range files {
		content, err := getContents(fileName)
		if err != nil {
			log.Printf("Error opening %q: %q\n", fileName, err)
			continue
		}
		contents <- content
	}
	close(contents)
	wg.Wait()
}

func serveContents(wg *sync.WaitGroup, conn io.Writer, contents <-chan io.Reader) {
	defer wg.Done()
	for content := range contents {
		_, err := io.Copy(conn, content)
		if err != nil {
			log.Printf("Error copying to socket: %q\n", err)
			continue
		}
	}
}

func getContents(fileName string) (io.Reader, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	r, w := io.Pipe()
	go func() {
		defer file.Close()
		defer w.Close()
		_, err := io.Copy(w, file)
		if err != nil {
			log.Printf("Error copying from %q: %q\n", fileName, err)
		}
	}()
	return r, nil
}
