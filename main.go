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
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var businessFolderId = "1GBmNGzO54HqjlWXrSN7Ds9zu_qJEx4gW"

var docIds = map[string]string{
	"stash":     "1Y2z7UZIIuvF3Twka6tXoovkbxyxXXz4qLnr9W43BIFs",
	"kubedb-30": "1n8zRoI5qjBaqa5hrogAey8OFd8-q7nCE9ysxwullb0g",
	"kubedb-40": "1s5751cd1SWZAy824njvTz2-iSC4V7NXRoFoCmZfoIcQ",
	"kubedb-45": "1VN3C_fDdUG_-zgFwvPkASVYzVmVr9E2Scv1Z2uqBRrY",
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

	tok, err := config.Exchange(oauth2.NoContext, authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	defer f.Close()
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
	defer f.Close()
	if err != nil {
		log.Fatalf("Unable to cache OAuth token: %v", err)
	}
	json.NewEncoder(f).Encode(token)
}

func main() {
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

	err = run(srvDoc, srvDrive)
	if err != nil {
		panic(err)
	}
}

type Info struct {
	Quote string

	Name        string
	Designation string
	Company     string
	Phone       string
	Email       string

	PrepDate   time.Time
	ExpiryDate time.Time
}

func (i Info) Date() map[string]string {
	data := map[string]string{}

	data["{{quote}}"] = i.Quote
	data["{{name}}"] = i.Name
	data["{{designation}}"] = i.Designation
	data["{{company}}"] = i.Company
	data["{{phone}}"] = i.Phone
	data["{{email}}"] = i.Email
	data["{{website}}"] = Domain(i.Email)
	data["{{prep-date}}"] = i.PrepDate.Format("Jan 2, 2006")
	data["{{expiry-date}}"] = i.ExpiryDate.Format("Jan 2, 2006")
	return data
}

func Domain(email string) string {
	parts := strings.Split(email, "@")
	return parts[len(parts)-1]
}

func run(srvDoc *docs.Service, srvDrive *drive.Service) error {
	info := Info{
		Quote:       "AC2012001",
		Name:        "Tamal Saha",
		Designation: "CEO",
		Company:     "AppsCode Inc.",
		Phone:       "+1(434)284-0668",
		Email:       "tamal@appscode.com",
		PrepDate:    time.Now(),
		ExpiryDate:  time.Now().Add(30 * 24 * time.Hour),
	}

	var domainFolderId string

	// https://developers.google.com/drive/api/v3/search-files
	q := fmt.Sprintf("name = '%s' and mimeType = 'application/vnd.google-apps.folder' and '%s' in parents", Domain(info.Email), businessFolderId)
	files, err := srvDrive.Files.List().Q(q).Spaces("drive").Do()
	if err != nil {
		return err
	}
	if len(files.Files) > 0 {
		fmt.Println("----------------")
		for _, f := range files.Files {
			fmt.Println(f.Id, f.Name)
		}
		fmt.Println("----------------")
		domainFolderId = files.Files[0].Id
	} else {
		// https://developers.google.com/drive/api/v3/folder#java
		folderMetadata := &drive.File{
			Name:     Domain(info.Email),
			MimeType: "application/vnd.google-apps.folder",
			Parents:  []string{businessFolderId},
		}
		folder, err := srvDrive.Files.Create(folderMetadata).Fields("id").Do()
		if err != nil {
			return err
		}
		domainFolderId = folder.Id
	}

	fmt.Println(domainFolderId)

	// https://developers.google.com/docs/api/how-tos/documents#copying_an_existing_document
	copyMetadata := &drive.File{
		Name:    fmt.Sprintf("%s QUOTE #%s", info.Company, info.Quote),
		Parents: []string{domainFolderId},
	}
	copyFile, err := srvDrive.Files.Copy(docIds["kubedb-45"], copyMetadata).Fields("id", "parents").Do()
	if err != nil {
		return err
	}
	fmt.Println(copyFile.Id)

	// https://developers.google.com/docs/api/how-tos/merge
	fields := info.Date()
	req := &docs.BatchUpdateDocumentRequest{
		Requests: make([]*docs.Request, 0, len(fields)),
	}
	for k, v := range fields {
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
	fmt.Println(">>>>>>>>>>>>>", doc.DocumentId)

	resp, err := srvDrive.Files.Export(doc.DocumentId, "application/pdf").Download()
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	io.Copy(&buf, resp.Body)
	err = ioutil.WriteFile("/home/tamal/Downloads/1a/test-quote.pdf", buf.Bytes(), 0644)
	if err != nil {
		return err
	}
	return nil
}
