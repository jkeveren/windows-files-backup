package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

type source struct {
	Path      string
	Blacklist []string
	info      os.FileInfo
}

const usage = "Usage: \"quickbooks-backup <directory to store backups>\""

var blacklist *regexp.Regexp

func main() {
	deleteOldBackups := true

	// validate arguments
	if len(os.Args) < 2 {
		log.Panicf("Not enough arguments. %s", usage)
	}

	// configure logging
	dstDirPath := os.Args[1]
	logFilePath := path.Join(dstDirPath, "log.txt")
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		log.Panic(err)
	}
	lw := io.MultiWriter(logFile, os.Stdout)
	l := log.New(lw, "", log.Lshortfile)

	dstDirPath, err = filepath.Abs(dstDirPath) // get absolute path after establishing logs so error can be read from file
	if err != nil {
		l.Panic(err)
	}

	// get sources
	sourcesJSON, err := ioutil.ReadFile(path.Join(dstDirPath, "sources.json"))
	if err != nil {
		l.Panic(err)
	}
	sources := make([]source, 0)
	err = json.Unmarshal(sourcesJSON, &sources)
	if err != nil {
		l.Print(err)
		deleteOldBackups = false
	}

	// transform and validate sources before creating backup zip
	for i := range sources {
		source := &sources[i]
		// Use absolute path for printing
		absPath, err := filepath.Abs(source.Path)
		if err != nil {
			l.Print(err)
			deleteOldBackups = false
		} else {
			source.Path = absPath
		}
		// validate that path exists
		info, err := os.Stat(source.Path)
		if err != nil {
			l.Print(err)
			deleteOldBackups = false
		} else {
			source.info = info
		}
	}

	// name destination
	t := time.Now().UTC()
	dstFileName := fmt.Sprintf("%d_UTC-%d-%d-%d.zip", t.Unix(), t.Year(), t.Month(), t.Day())
	backupDirPath := path.Join(dstDirPath, "backups")
	dstFilePath := path.Join(backupDirPath, dstFileName)

	// create zip
	err = os.Mkdir(backupDirPath, os.ModeDir)
	if err != nil && !os.IsExist(err) {
		l.Panic(err)
	}
	dstFile, err := os.Create(dstFilePath)
	if err != nil {
		l.Panic(err)
	}
	defer dstFile.Close()
	dstZip := zip.NewWriter(dstFile)
	defer dstZip.Close()

	// add files to zip
	for i, source := range sources {
		baseName := filepath.Base(source.Path)
		errs := addSrc(dstZip, source.Path, fmt.Sprintf("source-%d:-%s", i+1, baseName), source.Blacklist) // include number for simple collision prevention
		for _, err := range errs {
			l.Print(err)
			deleteOldBackups = false
		}
	}

	// delete old logs
	if !deleteOldBackups {
		l.Panic("Errors occurred. Old backups will not be deleted automatically.")
	}
	format := "Unable to delete old backups: %s"
	backupInfos, err := ioutil.ReadDir(backupDirPath)
	if err != nil {
		l.Panicf(format, err)
	}
	backupReg, err := regexp.Compile("^\\d{10}_UTC-\\d{4}-\\d{1,2}-\\d{1,2}")
	if err != nil {
		l.Printf(format, err)
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
		err := os.Remove(path.Join(backupDirPath, name))
		if err != nil {
			l.Print(err)
		}
	}

	l.Print("Done.")
}

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
