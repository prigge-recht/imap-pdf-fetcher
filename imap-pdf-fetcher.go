package main

import (
	"flag"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message"
	"github.com/joho/godotenv"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	loadEnv()
	setLogFile()

	var path string
	flag.StringVar(&path, "p", "", "Path of directory with PDF files to place.")
	flag.Parse()

	if path == "" {
		log.Fatal("Path is empty.")
	}

	fetchAttachments()
	ocrPDFs(path)
}

func loadEnv() {
	err := godotenv.Load()
	checkIfErrNil(err)

	if os.Getenv("SMTP_SERVER") == "" {
		log.Fatal("SMTP-SERVER must be provided.")
	}

	if os.Getenv("SMTP_USER") == "" {
		log.Fatal("SMTP_USER must be provided.")
	}

	if os.Getenv("SMTP_PASSWORD") == "" {
		log.Fatal("SMTP_PASSWORD must be provided.")
	}
}

func setLogFile() {
	file, _ := os.OpenFile("info.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	log.SetOutput(file)
}

func checkIfErrNil(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func fetchAttachments() {
	c, err := client.DialTLS(os.Getenv("SMTP_SERVER"), nil)
	checkIfErrNil(err)

	defer c.Logout()

	if err := c.Login(os.Getenv("SMTP_USER"), os.Getenv("SMTP_PASSWORD")); err != nil {
		log.Fatal(err)
	}
	log.Println("Connection established")

	mbox, err := c.Select("INBOX", false)
	checkIfErrNil(err)

	if mbox.Messages == 0 {
		log.Println("No messages in INBOX")
		return
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(1, mbox.Messages)

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)

	go func() {
		done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchRFC822}, messages)
	}()

	for msg := range messages {
		for _, r := range msg.Body {
			entity, err := message.Read(r)
			checkIfErrNil(err)

			multiPartReader := entity.MultipartReader()

			for e, err := multiPartReader.NextPart(); err != io.EOF; e, err = multiPartReader.NextPart() {
				kind, params, cErr := e.Header.ContentType()
				checkIfErrNil(cErr)

				if kind != "application/pdf" {
					continue
				}

				c, rErr := ioutil.ReadAll(e.Body)
				checkIfErrNil(rErr)

				log.Printf("Fetch %s", params["name"])

				err := os.MkdirAll("tmp/", os.ModePerm)

				checkIfErrNil(err)

				if fErr := ioutil.WriteFile("tmp/"+params["name"], c, 0777); fErr != nil {
					log.Fatal(fErr)
				}
			}
		}
	}

	if err := <-done; err != nil {
		log.Fatal(err)
	}

	_ = c.Move(seqset, "processed")
}

func ocrPDFs(path string) {
	var files []string

	root := "tmp"

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == ".pdf" {
			files = append(files, path)
		}
		return nil
	})

	checkIfErrNil(err)

	for _, file := range files {

		newFile := strings.TrimSuffix(file, filepath.Ext(file)) + "_ocr.pdf"

		app := "ocrmypdf"

		arg0 := file
		arg1 := newFile

		cmd := exec.Command(app, arg0, arg1)
		_, err := cmd.Output()

		checkIfErrNil(err)

		log.Println("OCR ready for " + file)

		err = os.Rename(newFile, filepath.Join(path, filepath.Base(newFile)))
		checkIfErrNil(err)

		log.Println("File moved to " + filepath.Join(path, filepath.Base(newFile)))

		err = os.Remove(file)
		checkIfErrNil(err)

		log.Println("File removed " + file)

	}
}
