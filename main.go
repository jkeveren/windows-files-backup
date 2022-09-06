package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type source struct {
	Path      string
	Blacklist []string
}

type Contact struct {
	Name string
	Email string
}

type configuration struct {
	Name              string
	ErrorContacts     []Contact
	SendGridAPIKey    string
	UseSendGrid       bool
	SalesScribeAPIKey string
	UseSalesScribe    bool
	Sources           []source
}

func main() {
	// Set up error handler
	e := errorHandler{
		logger: log.New(os.Stdout, "", 0),
	}
	var config configuration
	defer e.report(&config)

	// Validate CLI args
	if len(os.Args) < 2 {
		// Don't panic because no trace is required.
		e.print(errors.New("Not enough arguments. Usage: \"backup <directory to store backups>\""))
		return
	}

	dstDirPath := os.Args[1]

	// Configure logger
	l, err := configureLogger(dstDirPath)
	e.panicIfErr(err)
	e.logger = l

	// Get absolute path after establishing logs so error can be written to file.
	dstDirPath, err = filepath.Abs(dstDirPath)
	e.panicIfErr(err)

	// Parse config
	configJSON, err := ioutil.ReadFile(path.Join(dstDirPath, "config.json"))
	e.panicIfErr(err)
	err = json.Unmarshal(configJSON, &config)
	e.panicIfErr(err)

	// Create destination file name.
	t := time.Now().UTC()
	dstFileName := fmt.Sprintf("%d_UTC-%d-%d-%d.zip", t.Unix(), t.Year(), t.Month(), t.Day())
	backupsDirPath := path.Join(dstDirPath, "backups")
	dstFilePath := path.Join(backupsDirPath, dstFileName)

	// Create backup dir if not exist.
	err = os.Mkdir(backupsDirPath, os.ModeDir|os.ModePerm)
	if err != nil && !os.IsExist(err) {
		e.panic(err)
	}

	// Create destination file.
	dstFile, err := os.Create(dstFilePath)
	e.panicIfErr(err)
	defer dstFile.Close()
	dstZip := zip.NewWriter(dstFile)
	defer dstZip.Close()

	// Add sources to destination file.
	for i, source := range config.Sources {
		source.Path = filepath.Join(dstDirPath, source.Path)
		baseName := filepath.Base(source.Path)
		errs := addSrc(dstZip, source.Path, fmt.Sprintf("source-%d:-%s", i+1, baseName), source.Blacklist) // include number for simple collision prevention
		for _, err := range errs {
			e.print(err)
		}
	}

	// Delete old backups.
	if len(e.errs) > 0 {
		e.panic(errors.New("Errors occurred. Old backups will not be deleted automatically."))
	}
	format := "Unable to delete old backups: %s "
	backupInfos, err := ioutil.ReadDir(backupsDirPath)
	if err != nil {
		e.panic(errors.New(format + err.Error()))
	}
	backupReg, err := regexp.Compile("^\\d{10}_UTC-\\d{4}-\\d{1,2}-\\d{1,2}")
	if err != nil {
		e.panic(errors.New(format + err.Error()))
	}
	backupNames := make([]string, 0)
	for _, info := range backupInfos {
		name := info.Name()
		if backupReg.MatchString(name) {
			backupNames = append(backupNames, name)
		}
	}
	sort.Strings(backupNames)
	deleteCount := len(backupNames) - 3
	if deleteCount < 0 {
		deleteCount = 0
	}
	oldBackupNames := backupNames[:deleteCount]
	for _, name := range oldBackupNames {
		l.Printf("Deleting old backup %q", name)
		err := os.Remove(path.Join(backupsDirPath, name))
		e.printIfErr(err)
	}

	l.Print("Done.")
}

type errorHandler struct {
	logger *log.Logger
	errs   []error
}

func (e *errorHandler) print(err error) {
	e.errs = append(e.errs, err)
	e.logger.Print(err)
}

func (e *errorHandler) panic(err error) {
	e.errs = append(e.errs, err)
	e.logger.Panic(err)
}

func (e *errorHandler) printIfErr(err error) {
	if err != nil {
		e.print(err)
	}
}

func (e *errorHandler) panicIfErr(err error) {
	if err != nil {
		e.panic(err)
	}
}

// Reports errors via email.
func (e *errorHandler) report(config *configuration) {
	// Only report if errors occurred.
	if len(e.errs) == 0 {
		return
	}

	if config.SendGridAPIKey == "" {
		e.logger.Panic("No SendGrid API key for report email.")
	}

	// Concat all errors that occured.
	var errorsString string
	for _, err := range e.errs {
		errorsString += err.Error() + "\n"
	}
	message := fmt.Sprintf("Errors occured while backing up %s:\n%s", config.Name, errorsString)

	// Create SendGrid request body.
	requestBodyString := `{
		"personalizations": [{"to": [{
			"email": "james@keve.ren"
		}]}],
		"from": {"email": "james@keve.ren"},
		"subject": ` + strconv.Quote("Errors while backing up "+config.Name) + `,
		"content": [{
			"type": "text/plain",
			"value": ` + strconv.Quote(message) + `
		}]
	}`

	// Make SendGrid request.
	request, err := http.NewRequest("POST", "https://api.sendgrid.com/v3/mail/send", strings.NewReader(requestBodyString))
	if err != nil {
		e.logger.Panic(err)
	}
	request.Header.Set("authorization", "Bearer "+config.SendGridAPIKey)
	request.Header.Set("content-type", "application/json")
	httpClient := &http.Client{}
	response, err := httpClient.Do(request)
	if err != nil {
		e.logger.Panic(err)
	}
	// If status code is not 2xx.
	if response.StatusCode/100 != 2 {
		// Read body
		responseBody, err := ioutil.ReadAll(response.Body)
		if err != nil {
			// Not critical; use failover body.
			responseBody = []byte("Error retrieving response body")
		}
		// Print SendGrid error.
		e.logger.Panic(errors.New(fmt.Sprintf("SendGrid returned non-200 status code \"%d\".\n\nReponse body: \"%s\".\n\nRequest body: \"%s\"", response.StatusCode, string(responseBody), requestBodyString)))
	}
}

// Create logger that writes to file and stdout.
func configureLogger(dstDirPath string) (*log.Logger, error) {
	logFilePath := path.Join(dstDirPath, "log.txt")
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return nil, err
	}
	lw := io.MultiWriter(logFile, os.Stdout)
	l := log.New(lw, "", log.Ltime|log.Ldate|log.Lshortfile)
	return l, nil
}

// Backs up everything in `srcPath` to zip using `w`.
func addSrc(w *zip.Writer, srcPath, dstPath string, blacklist []string) []error {
	for _, pattern := range blacklist {
		match, err := filepath.Match(pattern, filepath.Base(srcPath))
		if err != nil {
			return []error{err}
		}
		if match {
			return []error{}
		}
	}
	info, err := os.Stat(srcPath)
	if err != nil {
		return []error{err}
	}
	if info.IsDir() {
		infos, err := ioutil.ReadDir(srcPath)
		if err != nil {
			return []error{err}
		}
		errs := make([]error, 0)
		for _, info := range infos {
			name := info.Name()
			childSrcPath := path.Join(srcPath, name)
			childDstPath := path.Join(dstPath, name)
			childErrs := addSrc(w, childSrcPath, childDstPath, blacklist)
			for _, err := range childErrs {
				errs = append(errs, err)
			}
		}
		return errs
	} else {
		src, err := os.Open(srcPath)
		if err != nil {
			return []error{err}
		}
		defer src.Close()
		dst, err := w.Create(dstPath)
		if err != nil {
			return []error{err}
		}
		_, err = io.Copy(dst, src)
		if err != nil {
			return []error{err}
		}
	}
	return []error{}
}
