package processor

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/J45k4/rtf"
	"github.com/PuerkitoBio/goquery"
	"github.com/ledongthuc/pdf"
	"github.com/nguyenthenguyen/docx"
	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
)

// ContentExtractor defines the interface for extracting text from various file formats.
type ContentExtractor interface {
	Extract(path string) (string, error)
}

// ExtractorRegistry maps file extensions to their respective extractors.
type ExtractorRegistry struct {
	extractors map[string]ContentExtractor
}

// NewExtractorRegistry initializes the registry with all supported extractors.
func NewExtractorRegistry() *ExtractorRegistry {
	r := &ExtractorRegistry{
		extractors: make(map[string]ContentExtractor),
	}

	// Standard Text/Markdown
	textExt := &TextExtractor{}
	r.Register(".txt", textExt)
	r.Register(".md", textExt)

	// Structured Data
	r.Register(".json", &JSONExtractor{})
	r.Register(".jsonl", &JSONExtractor{})
	r.Register(".csv", &CSVExtractor{Separator: ','})
	r.Register(".tsv", &CSVExtractor{Separator: '\t'})
	r.Register(".yaml", &YAMLExtractor{})
	r.Register(".yml", &YAMLExtractor{})

	// Rich Documents
	r.Register(".pdf", &PDFExtractor{})
	r.Register(".docx", &DocxExtractor{})
	r.Register(".xlsx", &XlsxExtractor{})
	r.Register(".html", &HTMLExtractor{})
	r.Register(".htm", &HTMLExtractor{})
	r.Register(".odt", &ODFExtractor{})
	r.Register(".ods", &ODFExtractor{})
	r.Register(".odp", &ODFExtractor{})
	r.Register(".rtf", &RTFExtractor{})

	// Developer Specific
	r.Register(".sql", &SQLExtractor{})
	r.Register(".xml", &XMLExtractor{})

	return r
}

func (r *ExtractorRegistry) Register(ext string, e ContentExtractor) {
	r.extractors[strings.ToLower(ext)] = e
}

func (r *ExtractorRegistry) Get(ext string) (ContentExtractor, bool) {
	e, ok := r.extractors[strings.ToLower(ext)]
	return e, ok
}

// --- Concrete Extractors ---

// TextExtractor handles plain text and markdown.
type TextExtractor struct{}

func (e *TextExtractor) Extract(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// JSONExtractor handles JSON and JSONL by extracting string values or pretty-printing.
type JSONExtractor struct{}

func (e *JSONExtractor) Extract(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// Try parsing as single JSON object first
	var obj interface{}
	if err := json.Unmarshal(data, &obj); err == nil {
		return fmt.Sprintf("%v", obj), nil
	}

	// Fallback to line-by-line for JSONL
	lines := strings.Split(string(data), "\n")
	var sb strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var lineObj interface{}
		if err := json.Unmarshal([]byte(line), &lineObj); err == nil {
			sb.WriteString(fmt.Sprintf("%v\n", lineObj))
		}
	}
	return sb.String(), nil
}

// CSVExtractor handles CSV and TSV by converting rows to labeled strings.
type CSVExtractor struct {
	Separator rune
}

func (e *CSVExtractor) Extract(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.Comma = e.Separator
	rows, err := reader.ReadAll()
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	var headers []string
	if len(rows) > 0 {
		headers = rows[0]
		rows = rows[1:]
	}

	for _, row := range rows {
		for i, val := range row {
			if i < len(headers) {
				sb.WriteString(fmt.Sprintf("%s: %s ", headers[i], val))
			} else {
				sb.WriteString(val + " ")
			}
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// YAMLExtractor handles YAML files.
type YAMLExtractor struct{}

func (e *YAMLExtractor) Extract(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var obj interface{}
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", obj), nil
}

// PDFExtractor handles PDF files using ledongthuc/pdf.
type PDFExtractor struct{}

func (e *PDFExtractor) Extract(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var sb strings.Builder
	totalPage := r.NumPage()
	for pageIndex := 1; pageIndex <= totalPage; pageIndex++ {
		p := r.Page(pageIndex)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			return "", err
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// DocxExtractor handles .docx files using nguyenthenguyen/docx.
type DocxExtractor struct{}

func (e *DocxExtractor) Extract(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	
	reader, err := docx.ReadDocxFromMemory(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	defer reader.Close()
	
	return reader.Editable().GetContent(), nil
}

// XlsxExtractor handles .xlsx files using excelize.
type XlsxExtractor struct{}

func (e *XlsxExtractor) Extract(path string) (string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var sb strings.Builder
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("Sheet: %s\n", sheet))
		for _, row := range rows {
			for _, colCell := range row {
				sb.WriteString(colCell + "\t")
			}
			sb.WriteString("\n")
		}
	}
	return sb.String(), nil
}

// HTMLExtractor handles HTML files using goquery.
type HTMLExtractor struct{}

func (e *HTMLExtractor) Extract(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		return "", err
	}

	// Remove scripts and styles
	doc.Find("script, style").Each(func(i int, s *goquery.Selection) {
		s.Remove()
	})

	return doc.Text(), nil
}

// SQLExtractor handles SQL dumps by extracting comments and INSERT values.
type SQLExtractor struct{}

func (e *SQLExtractor) Extract(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ODFExtractor handles OpenDocument files (.odt, .ods, .odp).
type ODFExtractor struct{}

func (e *ODFExtractor) Extract(path string) (string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	var contentFile *zip.File
	for _, f := range reader.File {
		if f.Name == "content.xml" {
			contentFile = f
			break
		}
	}

	if contentFile == nil {
		return "", fmt.Errorf("content.xml not found in ODF archive")
	}

	rc, err := contentFile.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	var sb strings.Builder
	decoder := xml.NewDecoder(rc)
	for {
		t, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch se := t.(type) {
		case xml.CharData:
			sb.WriteString(string(se))
			sb.WriteString(" ")
		}
	}

	return strings.TrimSpace(sb.String()), nil
}

// RTFExtractor handles Rich Text Format using J45k4/rtf.
type RTFExtractor struct{}

func (e *RTFExtractor) Extract(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// The library provides StripRichTextFormat
	return rtf.StripRichTextFormat(string(data)), nil
}

// XMLExtractor handles generic XML files.
type XMLExtractor struct{}

func (e *XMLExtractor) Extract(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var sb strings.Builder
	decoder := xml.NewDecoder(f)
	for {
		t, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch se := t.(type) {
		case xml.CharData:
			sb.WriteString(string(se))
			sb.WriteString(" ")
		}
	}
	return strings.TrimSpace(sb.String()), nil
}
