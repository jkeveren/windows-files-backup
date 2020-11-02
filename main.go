package main

import (
	"archive/zip"
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
	path string
	info os.FileInfo
}

const usage = "Usage: \"quickbooks-backup <directory to store backups> <space seperated list of directories or files to back up>\""

var blacklist *regexp.Regexp

func main() {
	// configure logging
	if len(os.Args) < 2 {
		log.Panicf("Not enough arguments. %s", usage)
	}
	dstDirPath := os.Args[1]
	logFilePath := path.Join(dstDirPath, "backup-log.txt")
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		log.Panic(err)
	}
	l := log.New(logFile, "", log.Lshortfile)

	deleteOldBackups := true

	blacklist, err = regexp.Compile("^[A-Z]:(/|\\\\)Users(/|\\\\).+?(/|\\\\)Documents(/|\\\\)My (Music|Pictures|Videos)+")
	if err != nil {
		l.Panic(err)
	}

	// validate args
	if len(os.Args) < 3 {
		l.Panicf("Not enough arguments. %s", usage)
	}

	// validate sources
	sources := make([]source, 0)
	for _, p := range os.Args[2:] {
		absPath, err := filepath.Abs(p) // Use absolute path for printing
		if err != nil {
			l.Print(err)
			deleteOldBackups = false
			continue
		}
		info, err := os.Stat(absPath)
		if err != nil {
			l.Print(err)
			deleteOldBackups = false
			continue
		}
		source := source{
			path: absPath,
			info: info,
		}
		sources = append(sources, source)
	}

	// name destination
	dstDirPath, err = filepath.Abs(dstDirPath)
	if err != nil {
		l.Panic(err)
	}
	t := time.Now().UTC()
	dstName := fmt.Sprintf("%d_UTC-%d-%d-%d.zip", t.Unix(), t.Year(), t.Month(), t.Day())
	dstFilePath := path.Join(dstDirPath, dstName)

	// create zip
	dstFile, err := os.Create(dstFilePath)
	if err != nil {
		l.Panic(err)
	}
	defer dstFile.Close()
	dstZip := zip.NewWriter(dstFile)
	defer dstZip.Close()

	// add files to zip
	for i, source := range sources {
		baseName := filepath.Base(source.path)
		errs := addSrc(dstZip, source.path, fmt.Sprintf("Source %d: %s", i+1, baseName)) // include number for simple collision prevention
		for _, err := range errs {
			l.Print(err)
			deleteOldBackups = false
		}
	}

	defer l.Print("Done.")

	// delete old logs
	if !deleteOldBackups {
		l.Panic("Errors occurred. Old backups will not be deleted automatically.")
	}
	format := "Unable to delete old backups: %s"
	dstDirInfos, err := ioutil.ReadDir(dstDirPath)
	if err != nil {
		l.Panicf(format, err)
	}
	backupReg, err := regexp.Compile("^\\d{10}_UTC-\\d{4}-\\d{1,2}-\\d{1,2}")
	if err != nil {
		l.Printf(format, err)
	}
	backupNames := make([]string, 0)
	for _, info := range dstDirInfos {
		name := info.Name()
		if backupReg.MatchString(name) {
			backupNames = append(backupNames, name)
		}
	}
	sort.Strings(backupNames)
	oldBackupNames := backupNames[:len(backupNames)-3]
	for _, name := range oldBackupNames {
		l.Printf("Deleting old backup %q", name)
		err := os.Remove(path.Join(dstDirPath, name))
		if err != nil {
			l.Print(err)
		}
	}

}

func addSrc(w *zip.Writer, srcPath, dstPath string) []error {
	if blacklist.MatchString(srcPath) {
		return []error{}
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
			childErrs := addSrc(w, childSrcPath, childDstPath)
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
