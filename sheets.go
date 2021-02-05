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
	"fmt"
	"strconv"
	"strings"
	"time"

	gdrive "gomodules.xyz/gdrive-utils"
)

func LogQuotation(si *gdrive.Spreadsheet, headers, data []string) (string, error) {
	const sheetName = "Quotation Log"

	sheetId, err := si.EnsureSheet(sheetName, headers)
	if err != nil {
		return "", err
	}

	lastQuote, err := si.FindEmptyCell(sheetName)
	if err != nil {
		return "", err
	}

	var quote string
	now := time.Now().UTC()
	if strings.HasPrefix(lastQuote, "AC") {
		y, err := strconv.Atoi(lastQuote[2:4])
		if err != nil {
			return "", fmt.Errorf("failed to detect YY from quote %s", lastQuote)
		}
		m, err := strconv.Atoi(lastQuote[4:6])
		if err != nil {
			return "", fmt.Errorf("failed to detect MM from quote %s", lastQuote)
		}
		sl, err := strconv.Atoi(lastQuote[6:])
		if err != nil {
			return "", fmt.Errorf("failed to detect Serial# from quote %s", lastQuote)
		}

		if (now.Year()-2000) == y && m == int(now.Month()) {
			quote = fmt.Sprintf("AC%02d%02d%03d", now.Year()-2000, now.Month(), sl+1)
		} else {
			quote = fmt.Sprintf("AC%02d%02d%03d", now.Year()-2000, now.Month(), 1)
		}
	} else {
		quote = fmt.Sprintf("AC%02d%02d%03d", now.Year()-2000, now.Month(), 1)
	}
	data[0] = quote

	return quote, si.AppendRowData(sheetId, data, false)
}
