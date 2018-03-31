package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/cmd/guru/serial"
)

type analyzeResult struct {
	path       string
	start      int64
	end        int64
	line       int64
	lineOffset int64
	identifier string
	definition serial.Definition
}

type resultChanStruct struct {
	path   string
	result []*analyzeResult
}

func (a *analyzeResult) ToString() string {
	return fmt.Sprintf("path: %s, start: %d, end: %d, line: %d, lineOffset: %d, identifier: %s\n %s \n", a.path, a.start, a.end, a.line, a.lineOffset, a.identifier, a.definition.ObjPos)
}

func analyzeFile(path string, info os.FileInfo, result chan resultChanStruct) {
	analyzeResults := make([]*analyzeResult, 0)
	lastIdentifierExist := false

	file, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer file.Close()
	fileBytes, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var line int64 = 1
	var totalReadLineBytesLen int64 = 1
	for i := int64(1); i <= info.Size(); i++ {
		if fileBytes[i-1] == '\n' {
			line++
			totalReadLineBytesLen = i
			continue
		}

		offset := "#" + strconv.FormatInt(i, 10)
		bytePos := path + ":" + offset
		lineOffset := i - totalReadLineBytesLen + 1

		// TODO: can we call go func directly?
		output, err := exec.Command("guru", "-json", "definition", bytePos).Output()
		if err != nil {
			if err.Error() == "exit status 1" {
				if lastIdentifierExist == true {
					analyzeResultsLen := len(analyzeResults)
					lastAnalyzeResult := analyzeResults[analyzeResultsLen-1]
					lastAnalyzeResult.end = i
					lastAnalyzeResult.identifier = string(fileBytes[lastAnalyzeResult.start:i])
				}

				lastIdentifierExist = false
				continue
			} else {
				fmt.Println(err)
				os.Exit(1)
			}
		}

		definition := serial.Definition{}
		err = json.Unmarshal(output, &definition)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if lastIdentifierExist == false {
			analyzeResults = append(analyzeResults, &analyzeResult{
				path:       path,
				start:      i,
				definition: definition,
				line:       line,
				lineOffset: lineOffset,
			})
			lastIdentifierExist = true
		}
	}

	result <- resultChanStruct{
		path:   path,
		result: analyzeResults,
	}
}

func buildLink(project string, ele *analyzeResult, fileBytes []byte) string {
	tag := html.EscapeString(string(fileBytes[ele.start:ele.end]))
	anchor := strings.Replace(ele.definition.ObjPos, project, "", -1)
	anchor = strings.Replace(anchor, os.Getenv("GOPATH"), "", -1)

	if ele.definition.ObjPos == ele.path+":"+strconv.FormatInt(ele.line, 10)+":"+strconv.FormatInt(ele.lineOffset, 10) {
		return "<a name='" + anchor + "'>" + tag + "</a>"
	}

	return "<a href='#" + anchor + "'>" + tag + "</a>"
}

func buildHTML(file *os.File, project string, path string, analyzeResultsPtr *[]*analyzeResult) (*string, error) {
	var htmlContent = ""

	analyzeResults := *analyzeResultsPtr

	fileBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lastEndAt int64
	for _, ele := range analyzeResults {
		text := html.EscapeString(string(fileBytes[lastEndAt:ele.start]))
		text = strings.Replace(text, " ", "&nbsp;", -1)
		htmlContent += strings.Replace(text, "\t", "&nbsp;&nbsp;&nbsp;&nbsp;", -1)
		htmlContent += buildLink(project, ele, fileBytes)
		lastEndAt = ele.end
	}

	analyzeResultsLen := len(analyzeResults)
	if analyzeResultsLen > 0 {
		lastAnalyzeResult := analyzeResults[analyzeResultsLen-1]
		htmlContent += html.EscapeString(string(fileBytes[lastAnalyzeResult.end:]))
	}

	htmlContent = strings.Replace(htmlContent, "\n", "<br />", -1)
	return &htmlContent, nil
}

func main() {
	project := os.Args[1]
	targetFilePath := os.Args[2]

	file, err := os.Create(targetFilePath)
	defer file.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	resultChan := make(chan resultChanStruct)
	goroutineCount := 0

	walk := func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		go analyzeFile(path, info, resultChan)
		goroutineCount++

		return nil
	}

	filepath.Walk(project, walk)

	file.WriteString("<!DOCTYPE html><html><body>")

	for goroutineCount > 0 {
		select {
		case fileResult := <-resultChan:
			htmlContent, err := buildHTML(file, project, fileResult.path, &fileResult.result)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			file.WriteString("<h3>" + strings.Replace(fileResult.path, project, "", -1) + "</h3>")
			file.WriteString(*htmlContent)
			file.WriteString("<hr />")
			file.WriteString("<br />")
			goroutineCount--
		}
	}

	file.WriteString("</body></html>")
}
