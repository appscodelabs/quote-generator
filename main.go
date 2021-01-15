/*
Copyright AppsCode Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/davegardnerisme/phonegeocode"
	"github.com/gobuffalo/flect"
	flag "github.com/spf13/pflag"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	. "gomodules.xyz/email-providers"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var businessFolderId = "1MW9ElMPDupVRohXqit-j6Wls-Cvq7DmN"

var templateIds = map[string]string{
	"stash-50":  "1EXMmcztXGb-EOrebHCrPrhFwQuRB0RpTl0UVeMtcMNk",
	"stash-100": "1Y2z7UZIIuvF3Twka6tXoovkbxyxXXz4qLnr9W43BIFs",
	"kubedb-30": "1n8zRoI5qjBaqa5hrogAey8OFd8-q7nCE9ysxwullb0g",
	"kubedb-40": "1s5751cd1SWZAy824njvTz2-iSC4V7NXRoFoCmZfoIcQ",
	"kubedb-45": "1VN3C_fDdUG_-zgFwvPkASVYzVmVr9E2Scv1Z2uqBRrY",
	"kubedb-cluster-edu": "18niPAUxB0OzsWTSln2OYuMqlXvHidozquqVwhtaFKYg",
}

var (
	parentFolderId     string
	templateDocId      string
	outDir             string
	replacementInput   map[string]string
	replacements       map[string]string
	email              string
	quote              string
	quoteSpreadsheetId = "1evwv2ON94R38M-Lkrw8b6dpVSkRYHUWsNOuI7X0_-zA"
)

func init() {
	flag.StringVar(&parentFolderId, "parent-folder-id", businessFolderId, "Parent folder id where generated docs will be stored under a folder with matching email domain")
	flag.StringVar(&templateDocId, "template-doc-id", "", "Template document id")
	flag.StringVar(&outDir, "out-dir", filepath.Join("/personal", "AppsCode", "quotes"), "Path to directory where output files are stored")
	flag.StringToStringVar(&replacementInput, "data", nil, "key-value pairs for text replacement")
	flag.StringVar(&quoteSpreadsheetId, "spreadsheet-id", quoteSpreadsheetId, "Google Spreadsheet Id used to store quotation log")
}

// Retrieves a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Requests a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	defer func() {
		Must(f.Close())
	}()
	if err != nil {
		return nil, err
	}
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	defer func() {
		Must(f.Close())
	}()
	if err != nil {
		log.Fatalf("Unable to cache OAuth token: %v", err)
	}
	Must(json.NewEncoder(f).Encode(token))
}

func main() {
	flag.Parse()

	if parentFolderId == "" {
		panic("missing parent folder id")
	}
	if templateDocId == "" {
		panic("missing template doc id")
	}
	if id, ok := templateIds[templateDocId]; ok {
		templateDocId = id
	}

	replacements = map[string]string{}
	for k, v := range replacementInput {
		if strings.HasPrefix(k, "{{") && strings.HasSuffix(k, "}}") {
			replacements[k] = v
			continue
		}
		k = strings.Trim(k, "{}")
		k = flect.Dasherize(k)
		replacements[fmt.Sprintf("{{%s}}", k)] = v
	}
	if v, ok := replacements["{{email}}"]; !ok {
		panic("missing email")
	} else {
		email = v
	}
	if IsPublicEmail(email) {
		replacements["{{website}}"] = ""
	} else {
		replacements["{{website}}"] = Domain(email)
	}

	if v, ok := replacements["{{phone}}"]; ok {
		replacements["{{tel}}"] = v
	}
	if v, ok := replacements["{{tel}}"]; ok {
		tel := SanitizeTelNumber(v)
		if !strings.HasPrefix(tel, "+") && len(tel) == 10 {
			tel = "+1" + tel
		}
		replacements["{{tel}}"] = tel
		if cc, err := phonegeocode.New().Country(tel); err == nil {
			replacements["{{country}}"] = cc
		}
	}

	now := time.Now()
	replacements["{{prep-date}}"] = now.Format("Jan 2, 2006")
	replacements["{{expiry-date}}"] = now.Add(30 * 24 * time.Hour).Format("Jan 2, 2006")

	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b,
		"https://www.googleapis.com/auth/documents",
		"https://www.googleapis.com/auth/documents.readonly",
		"https://www.googleapis.com/auth/drive",
		"https://www.googleapis.com/auth/drive.file",
		"https://www.googleapis.com/auth/drive.metadata",
		"https://www.googleapis.com/auth/drive.metadata.readonly",
		"https://www.googleapis.com/auth/drive.readonly",
		"https://www.googleapis.com/auth/spreadsheets",
		"https://www.googleapis.com/auth/spreadsheets.readonly",
	)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srvDrive, err := drive.NewService(context.TODO(), option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Docs client: %v", err)
	}

	srvDoc, err := docs.NewService(context.TODO(), option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Docs client: %v", err)
	}

	srvSheet, err := NewSpreadsheet(quoteSpreadsheetId, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Sheets client: %v", err)
	}
	quote, err = srvSheet.Append([]string{
		"Quotation #",
		"Name",
		"Designation",
		"Email",
		"Telephone",
		"Company",
		"Website",
		"Country",
		"Pricing Template",
		"Preparation Date",
		"Expiration Date",
	}, []string{
		"AC_DETECT_QUOTE",
		replacements["{{name}}"],
		replacements["{{designation}}"],
		replacements["{{email}}"],
		replacements["{{tel}}"],
		replacements["{{company}}"],
		replacements["{{website}}"],
		replacements["{{country}}"],
		templateDocId,
		replacements["{{prep-date}}"],
		replacements["{{expiry-date}}"],
	})
	if err != nil {
		log.Fatalf("Unable to append quotation: %v", err)
	}
	replacements["{{quote}}"] = quote

	err = run(srvDoc, srvDrive)
	if err != nil {
		panic(err)
	}
}

func FolderName(email string) string {
	if IsPublicEmail(email) {
		return email
	}
	parts := strings.Split(email, "@")
	return parts[len(parts)-1]
}

func Must(err error) {
	if err != nil {
		panic(err)
	}
}

func run(srvDoc *docs.Service, srvDrive *drive.Service) error {
	var domainFolderId string

	// https://developers.google.com/drive/api/v3/search-files
	q := fmt.Sprintf("name = '%s' and mimeType = 'application/vnd.google-apps.folder' and '%s' in parents", FolderName(email), parentFolderId)
	files, err := srvDrive.Files.List().Q(q).Spaces("drive").Do()
	if err != nil {
		return err
	}
	if len(files.Files) > 0 {
		domainFolderId = files.Files[0].Id
	} else {
		// https://developers.google.com/drive/api/v3/folder#java
		folderMetadata := &drive.File{
			Name:     FolderName(email),
			MimeType: "application/vnd.google-apps.folder",
			Parents:  []string{parentFolderId},
		}
		folder, err := srvDrive.Files.Create(folderMetadata).Fields("id").Do()
		if err != nil {
			return err
		}
		domainFolderId = folder.Id
	}
	fmt.Println("Using domain folder id:", domainFolderId)

	// https://developers.google.com/docs/api/how-tos/documents#copying_an_existing_document
	docName := fmt.Sprintf("%s QUOTE #%s", FolderName(email), quote)
	copyMetadata := &drive.File{
		Name:    docName,
		Parents: []string{domainFolderId},
	}
	copyFile, err := srvDrive.Files.Copy(templateDocId, copyMetadata).Fields("id", "parents").Do()
	if err != nil {
		return err
	}
	fmt.Println("doc id:", copyFile.Id)

	// https://developers.google.com/docs/api/how-tos/merge
	req := &docs.BatchUpdateDocumentRequest{
		Requests: make([]*docs.Request, 0, len(replacements)),
	}
	for k, v := range replacements {
		req.Requests = append(req.Requests, &docs.Request{
			ReplaceAllText: &docs.ReplaceAllTextRequest{
				ContainsText: &docs.SubstringMatchCriteria{
					MatchCase: true,
					Text:      k,
				},
				ReplaceText: v,
			},
		})
	}
	doc, err := srvDoc.Documents.BatchUpdate(copyFile.Id, req).Do()
	if err != nil {
		return err
	}

	resp, err := srvDrive.Files.Export(doc.DocumentId, "application/pdf").Download()
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return err
	}
	filename := filepath.Join(outDir, FolderName(email), docName+".pdf")
	err = os.MkdirAll(filepath.Dir(filename), 0755)
	if err != nil {
		return err
	}
	fmt.Println("writing file:", filename)
	err = ioutil.WriteFile(filename, buf.Bytes(), 0644)
	if err != nil {
		return err
	}
	return nil
}

func SanitizeTelNumber(tel string) string {
	var buf bytes.Buffer
	for _, r := range tel {
		if r == '+' || (r >= '0' && r <= '9') {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
