package main

import (
	"context"
	"io/ioutil"
	"math/rand"

	"errors"
	"fmt"
	"io"

	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/procyon-projects/chrono"
	"github.com/schollz/progressbar/v3"
	"github.com/yeka/zip"
)

func main() {
	fmt.Println(`
 __  __      _    _                 
|  \/  |__ _| |__| |_  __ _ _ _ ___ 
| |\/| / _' | (_-< ' \/ _' | '_/ -_)
|_|  |_\__,_|_/__/_||_\__,_|_| \___|								   
 `)

	fetchMalware()

	taskScheduler := chrono.NewDefaultTaskScheduler()
	_, taskErr := taskScheduler.ScheduleWithCron(func(ctx context.Context) {
		fetchMalware()
	}, "0 30 0 * * *", chrono.WithLocation("UTC"))
	if taskErr != nil {
		fmt.Printf("Couldn't schedule fetch task\n")
		os.Exit(1)
	}

	http.HandleFunc("/", sendMalware)

	fmt.Printf("Starting server...\n")

	err := http.ListenAndServe(":3456", nil)

	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("Server stopped\n")
	} else if err != nil {
		fmt.Printf("Starting server failed: %s\n", err)
		os.Exit(1)
	}
}

func sendMalware(writer http.ResponseWriter, request *http.Request) {
	dir, dirErr := ioutil.ReadDir("samples")
	if dirErr != nil {
		fmt.Printf("Couldn't read samples directory\n")
		io.WriteString(writer, "ERROR")
		return
	}

	fileName := dir[rand.Intn(len(dir))].Name()

	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))

	http.ServeFile(writer, request, fmt.Sprintf("samples/%s", fileName))
}

func fetchMalware() {
	currentDate := time.Now().UTC()

	fmt.Printf("Current:	%s\n", currentDate)

	currentDate = currentDate.AddDate(0, 0, -1)
	dlUrl := fmt.Sprintf("https://datalake.abuse.ch/malware-bazaar/daily/%04d-%02d-%02d.zip", currentDate.Year(), currentDate.Month(), currentDate.Day())

	fmt.Printf("Downloading:	%s\n", dlUrl)

	resp, httpErr := http.Get(dlUrl)
	if httpErr != nil {
		fmt.Printf("File not found\n")
		return
	}
	defer resp.Body.Close()

	{
		file, fileErr := os.Create(fmt.Sprintf("%04d-%02d-%02d.zip", currentDate.Year(), currentDate.Month(), currentDate.Day()))
		if fileErr != nil {
			fmt.Printf("Can't create file\n")
			return
		}
		defer file.Close()

		bar := progressbar.DefaultBytes(
			resp.ContentLength,
			"downloading",
		)
		_, dlErr := io.Copy(io.MultiWriter(file, bar), resp.Body)
		if dlErr != nil {
			fmt.Printf("Error downloading file\n")
			return
		}
	}

	cleanErr := os.RemoveAll("samples")
	if cleanErr != nil {
		fmt.Printf("Can't remove old files\n")
	}

	zipReader, zipErr := zip.OpenReader(fmt.Sprintf("%04d-%02d-%02d.zip", currentDate.Year(), currentDate.Month(), currentDate.Day()))
	if zipErr != nil {
		fmt.Printf("Can't open zip\n")
		return
	}
	defer zipReader.Close()

	dirErr := os.MkdirAll("samples/", os.ModePerm)
	if dirErr != nil {
		fmt.Printf("Can't create directory\n")
		return
	}

	for _, zipEntry := range zipReader.File {
		if !strings.HasSuffix(zipEntry.Name, ".exe") {
			continue
		}

		zipEntry.SetPassword("infected")

		filePath := filepath.Join(".", "samples", zipEntry.Name)

		zipFile, zipFileErr := zipEntry.Open()
		if zipFileErr != nil {
			fmt.Printf("Can't open zip entry: %s\n", zipEntry.Name)
			continue
		}
		defer zipFile.Close()

		dstFile, dstErr := os.Create(filePath)
		if dstErr != nil {
			fmt.Printf("Can't create file: %s\n", filePath)
			continue
		}
		defer dstFile.Close()

		bar := progressbar.DefaultBytes(
			resp.ContentLength,
			zipEntry.Name,
		)

		_, unErr := io.Copy(io.MultiWriter(dstFile, bar), zipFile)
		if unErr != nil {
			fmt.Printf("Error unpacking file\n")
		}
	}

	os.Remove(fmt.Sprintf("%04d-%02d-%02d.zip", currentDate.Year(), currentDate.Month(), currentDate.Day()))
}
