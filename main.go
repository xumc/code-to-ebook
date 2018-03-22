package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

func (a *analyzeResult) ToString() string {
	return fmt.Sprintf("path: %s, start: %d, end: %d, line: %d, lineOffset: %d, identifier: %s\n %s \n", a.path, a.start, a.end, a.line, a.lineOffset, a.identifier, a.definition.ObjPos)
}

func analyzeFile(path string, info os.FileInfo) (*[]*analyzeResult, error) {
	analyzeResults := make([]*analyzeResult, 0)
	lastIdentifierExist := false

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fileBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var line int64 = 1
	var totalReadLineBytesLen int64 = 1
	for i := int64(1); i <= info.Size(); i++ {
		if fileBytes[i-1] == '\n' {
			line++
			totalReadLineBytesLen = i
		}

		offset := "#" + strconv.FormatInt(i, 10)
		bytePos := path + ":" + offset
		lineOffset := i - totalReadLineBytesLen + 1

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
				return nil, err
			}
		}

		definition := serial.Definition{}
		err = json.Unmarshal(output, &definition)
		if err != nil {
			return nil, err
		}

		if lastIdentifierExist == true {
			matched, err := regexp.Match("[0-9A-Za-z_]", []byte{fileBytes[i-1]})
			if err != nil {
				fmt.Println(err)
			}
			if matched {
				continue
			}
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

	return &analyzeResults, nil
}

func buildLink(project string, ele *analyzeResult, fileBytes []byte) string {
	tag := string(fileBytes[ele.start:ele.end])
	anchor := strings.Replace(ele.definition.ObjPos, project, "", -1)
	anchor = strings.Replace(anchor, os.Getenv("GOPATH"), "", -1)

	if ele.definition.ObjPos == ele.path+":"+strconv.FormatInt(ele.line, 10)+":"+strconv.FormatInt(ele.lineOffset, 10) {
		return "<a name='" + anchor + "'>" + tag + "</a>"
	}

	return "<a href='#" + anchor + "'>" + tag + "</a>"
}

func buildHTML(project string, path string, analyzeResultsPtr *[]*analyzeResult) (*string, error) {
	var html = "<!DOCTYPE html><html><body>"

	analyzeResults := *analyzeResultsPtr

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fileBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lastEndAt int64
	for _, ele := range analyzeResults {
		text := string(fileBytes[lastEndAt:ele.start])
		text = strings.Replace(text, " ", "&nbsp;", -1)
		html += strings.Replace(text, "\t", "&nbsp;&nbsp;&nbsp;&nbsp;", -1)
		html += buildLink(project, ele, fileBytes)
		lastEndAt = ele.end
	}

	analyzeResultsLen := len(analyzeResults)
	if analyzeResultsLen > 0 {
		lastAnalyzeResult := analyzeResults[analyzeResultsLen-1]
		html += string(fileBytes[lastAnalyzeResult.end:])
	}

	html += "</body></html>"

	html = strings.Replace(html, "\n", "<br />", -1)
	return &html, nil
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

	walk := func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		analyzeResults, err := analyzeFile(path, info)
		if err != nil {
			return nil
		}

		html, err := buildHTML(project, path, analyzeResults)
		if err != nil {
			return nil
		}

		file.WriteString("<h5>" + strings.Replace(path, project, "", -1) + "</h5>")
		file.WriteString("<hr />")
		file.WriteString(*html)
		file.WriteString("<br /> <br />")

		return nil
	}

	filepath.Walk(project, walk)
}
