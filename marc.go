package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
    _ "embed"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

type Page struct {
	Meta    map[string]string
	Text    []byte
	Url     string
	HTML    template.HTML
	AbsPath string
	RelPath string
}

func (p Page) sortKey() string {
	return p.Meta["date"]
}

var dateFormats = map[string]string{
	"rfc822":     time.RFC822,
	"yyyy-mm-dd": "2006-01-02",
	"shortdate":  "02 Jan 2006",
}

var funcs = template.FuncMap{
	"dateformat": func(src, dst, input string) (string, error) {
		srcfmt, ok := dateFormats[src]
		if !ok {
			return "", fmt.Errorf("unknown date format: %s", src)
		}

		dstfmt, ok := dateFormats[dst]
		if !ok {
			return "", fmt.Errorf("unknown date format: %s", dst)
		}
		t, _ := time.Parse(srcfmt, input)
		return t.Format(dstfmt), nil
	},
}

func readMeta(b []byte) (map[string]string, []byte) {
	delim := []byte("---")
	if len(b) < 3 || !bytes.Equal(b[:3], delim) {
		return nil, b
	}
	i := bytes.Index(b[3:], delim)
	if i == -1 {
		return nil, b
	}

	meta := make(map[string]string)
	for _, line := range strings.Split(string(b[3:i+3]), "\n") {
		if keyval := strings.SplitN(line, ":", 2); len(keyval) == 2 {
			key := strings.TrimSpace(keyval[0])
			val := strings.TrimSpace(keyval[1])
			meta[key] = val
		}
	}
	return meta, b[i+6:]
}

type Pages []Page

func (p Pages) Len() int { return len(p) }
func (p Pages) Less(i, j int) bool {
	return strings.Compare(p[i].sortKey(), p[j].sortKey()) > 0
}
func (p Pages) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func readPage(abspath string, siteDir string) Page {
	text, err := os.ReadFile(abspath)
	if err != nil {
		log.Fatal("failed to read page:", err)
	}
	meta, text := readMeta(text)
	relpath, err := filepath.Rel(siteDir, abspath)
	if err != nil {
		log.Fatal("failed to get page extension:", err)
	}

	url := strings.TrimSuffix(relpath, filepath.Ext(relpath)) + ".html"
	url = strings.TrimSuffix(url, "index.html")

	page := Page{
		Meta:    meta,
		Url:     url,
		AbsPath: abspath,
		RelPath: relpath,
		Text:    text,
	}
	return page
}


//go:embed github-markdown.css
var defaultCSS string
//go:embed github-markdown.tmpl
var defaultHTML string

func readTmpl(siteDir string) *template.Template {
    tmplBase := template.New("base").Funcs(funcs)
	tmplPath := filepath.Join(siteDir, "base.tmpl")
	if tmplText, err := os.ReadFile(tmplPath); err == nil {
        if tmpl, err := tmplBase.Parse(string(tmplText)); err == nil {
            return tmpl
        }
    }
    text := strings.Replace(defaultHTML, "STYLE_PLACEHOLDER", defaultCSS, 1)
    return template.Must(tmplBase.Parse(text))
}

func main() {
	log.SetFlags(0)
	if len(os.Args) != 2 {
		log.Fatalf("usage: %s /path/to/site\n", os.Args[0])
		return
	}

	siteDir := os.Args[1]
    baseTmpl := readTmpl(siteDir)

	pages := make(Pages, 0)
	filepath.WalkDir(siteDir, func(path string, d fs.DirEntry, err error) error {
		if filepath.Ext(path) != ".md" {
			return nil
		}
		page := readPage(path, siteDir)
		pages = append(pages, page)
		return nil
	})
	sort.Stable(pages)

	md := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)

	var buf bytes.Buffer
	for _, page := range pages {
		ext := filepath.Ext(page.AbsPath)
		outPath := strings.TrimSuffix(page.AbsPath, ext) + ".html"
		log.Println("*", outPath)

		body := page.Text

		buf.Reset()
		err := md.Convert(body, &buf)
		if err != nil {
			log.Fatal("failed to convert markdown:", err)
		}
		body = buf.Bytes()

		page.HTML = template.HTML(body)

		buf.Reset()
		err = baseTmpl.Execute(&buf, map[string]interface{}{
			"Page":  page,
			"Pages": pages,
		})
		if err != nil {
			log.Fatal("failed to render page:", err)
		}
		body = buf.Bytes()
		err = os.WriteFile(outPath, body, 0600)
		if err != nil {
			log.Fatal("failed to write file:", err)
		}
	}
}
