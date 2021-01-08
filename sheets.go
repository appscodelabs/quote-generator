package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type Spreadsheet struct {
	srv            *sheets.Service
	SpreadSheetId  string
	CurrentSheetID int64
}

func NewSpreadsheet(spreadsheetId string, opts ...option.ClientOption) (*Spreadsheet, error) {
	// Set env GOOGLE_APPLICATION_CREDENTIALS to service account json path
	srv, err := sheets.NewService(context.TODO(), opts...)
	if err != nil {
		return nil, err
	}

	return &Spreadsheet{
		srv:           srv,
		SpreadSheetId: spreadsheetId,
	}, nil
}

// ref: https://developers.google.com/sheets/api/guides/batchupdate
func (si *Spreadsheet) updateRowData(row int64, data []string, formatCell bool) error {
	var format *sheets.CellFormat

	if formatCell {
		// for updating header color and making it bold
		format = &sheets.CellFormat{
			TextFormat: &sheets.TextFormat{
				Bold: true,
			},
			BackgroundColor: &sheets.Color{
				Alpha: 1,
				Blue:  149.0 / 255.0,
				Green: 226.0 / 255.0,
				Red:   239.0 / 255.0,
			},
		}
	}

	vals := make([]*sheets.CellData, 0, len(data))
	for i := range data {
		vals = append(vals, &sheets.CellData{
			UserEnteredFormat: format,
			UserEnteredValue: &sheets.ExtendedValue{
				StringValue: &data[i],
			},
		})
	}

	req := []*sheets.Request{
		{
			UpdateCells: &sheets.UpdateCellsRequest{
				Fields: "*",
				Start: &sheets.GridCoordinate{
					ColumnIndex: 0,
					RowIndex:    row,
					SheetId:     si.CurrentSheetID,
				},
				Rows: []*sheets.RowData{
					{
						Values: vals,
					},
				},
			},
		},
	}
	_, err := si.srv.Spreadsheets.BatchUpdate(si.SpreadSheetId, &sheets.BatchUpdateSpreadsheetRequest{
		IncludeSpreadsheetInResponse: false,
		Requests:                     req,
		ResponseIncludeGridData:      false,
	}).Do()
	if err != nil {
		return fmt.Errorf("unable to update: %v", err)
	}

	return nil
}

// ref: https://developers.google.com/sheets/api/guides/batchupdate
func (si *Spreadsheet) appendRowData(data []string, formatCell bool) error {
	var format *sheets.CellFormat

	if formatCell {
		// for updating header color and making it bold
		format = &sheets.CellFormat{
			TextFormat: &sheets.TextFormat{
				Bold: true,
			},
			BackgroundColor: &sheets.Color{
				Alpha: 1,
				Blue:  149.0 / 255.0,
				Green: 226.0 / 255.0,
				Red:   239.0 / 255.0,
			},
		}
	}

	vals := make([]*sheets.CellData, 0, len(data))
	for i := range data {
		vals = append(vals, &sheets.CellData{
			UserEnteredFormat: format,
			UserEnteredValue: &sheets.ExtendedValue{
				StringValue: &data[i],
			},
		})
	}

	req := []*sheets.Request{
		{
			AppendCells: &sheets.AppendCellsRequest{
				SheetId: si.CurrentSheetID,
				Fields:  "*",
				Rows: []*sheets.RowData{
					{
						Values: vals,
					},
				},
			},
		},
	}
	_, err := si.srv.Spreadsheets.BatchUpdate(si.SpreadSheetId, &sheets.BatchUpdateSpreadsheetRequest{
		IncludeSpreadsheetInResponse: false,
		Requests:                     req,
		ResponseIncludeGridData:      false,
	}).Do()
	if err != nil {
		return fmt.Errorf("unable to update: %v", err)
	}

	return nil
}

func (si *Spreadsheet) getSheetId(name string) (int64, error) {
	resp, err := si.srv.Spreadsheets.Get(si.SpreadSheetId).Do()
	if err != nil {
		return -1, fmt.Errorf("unable to retrieve data from sheet: %v", err)
	}
	var id int64
	for _, sheet := range resp.Sheets {
		if sheet.Properties.Title == name {
			id = sheet.Properties.SheetId
		}

	}

	return id, nil
}

func (si *Spreadsheet) addNewSheet(name string) error {
	req := sheets.Request{
		AddSheet: &sheets.AddSheetRequest{
			Properties: &sheets.SheetProperties{
				Title: name,
			},
		},
	}

	rbb := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{&req},
	}

	_, err := si.srv.Spreadsheets.BatchUpdate(si.SpreadSheetId, rbb).Context(context.Background()).Do()
	if err != nil {
		return err
	}

	return nil
}

func (si *Spreadsheet) ensureSheet(name string, headers []string) (int64, error) {
	id, err := si.getSheetId(name)
	if err != nil {
		return 0, err
	}

	if id == 0 {
		err = si.addNewSheet(name)
		if err != nil {
			return 0, err
		}

		id, err = si.getSheetId(name)
		if err != nil {
			return 0, err
		}

		si.CurrentSheetID = id

		err = si.ensureHeader(headers)
		if err != nil {
			return 0, err
		}

		return id, nil
	}

	si.CurrentSheetID = id
	return id, nil
}

func (si *Spreadsheet) ensureHeader(headers []string) error {
	return si.updateRowData(0, headers, true)
}

func (si *Spreadsheet) Append(headers, data []string) (string, error) {
	_, err := si.ensureSheet("Quotation Log", headers)
	if err != nil {
		return "", err
	}

	row, err := si.findEmptyCell()
	if err != nil {
		return "", err
	}

	lastQuote, err := si.getCellData(row-1, 0)
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

	return "", si.appendRowData(data, false)
}

func (si *Spreadsheet) findEmptyCell() (int64, error) {
	resp, err := si.srv.Spreadsheets.GetByDataFilter(si.SpreadSheetId, &sheets.GetSpreadsheetByDataFilterRequest{
		IncludeGridData: true,
	}).Do()
	if err != nil {
		return 0, fmt.Errorf("unable to retrieve data from sheet: %v", err)
	}

	for _, s := range resp.Sheets {
		if s.Properties.SheetId == si.CurrentSheetID {
			return int64(len(s.Data[0].RowData)), nil
		}
	}

	return 0, errors.New("no empty cell found")
}

func (si *Spreadsheet) getCellData(row, column int64) (string, error) {
	resp, err := si.srv.Spreadsheets.GetByDataFilter(si.SpreadSheetId, &sheets.GetSpreadsheetByDataFilterRequest{
		IncludeGridData: true,
	}).Do()
	if err != nil {
		return "", fmt.Errorf("unable to retrieve data from sheet: %v", err)
	}

	var val string

	for _, s := range resp.Sheets {
		if s.Properties.SheetId == si.CurrentSheetID {
			val = s.Data[0].RowData[row].Values[column].FormattedValue
		}
	}

	return val, nil
}