package goscraper

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

var (
	EscapedFragment string = "_escaped_fragment_="
	fragmentRegexp         = regexp.MustCompile("#!(.*)")
)

type scrapeSettings struct {
	userAgent         string
	maxDocumentLength int64
	url               string
	maxRedirect       int
}

type ScrapeBuilder interface {
	SetUserAgent(string) ScrapeBuilder
	SetMaxDocumentLength(int64) ScrapeBuilder
	SetUrl(string) ScrapeBuilder
	SetMaxRedirect(int) ScrapeBuilder
	Build() (ScrapeService, error)
}

type scrapeBuilder struct {
	scrapeSettings scrapeSettings
}

func (b *scrapeBuilder) Build() (ScrapeService, error) {
	u, err := url.Parse(b.scrapeSettings.url)
	if err != nil {
		return nil, err
	}
	return &Scraper{
		Url:         u,
		MaxRedirect: b.scrapeSettings.maxRedirect,
		Options: ScraperOptions{
			MaxDocumentLength: b.scrapeSettings.maxDocumentLength,
			UserAgent:         b.scrapeSettings.userAgent,
		}}, nil
}

func (b *scrapeBuilder) SetUrl(s string) ScrapeBuilder {
	b.scrapeSettings.url = s
	return b
}

func (b *scrapeBuilder) SetMaxRedirect(i int) ScrapeBuilder {
	b.scrapeSettings.maxRedirect = i
	return b
}

func (b *scrapeBuilder) SetMaxDocumentLength(maxDocLength int64) ScrapeBuilder {
	b.scrapeSettings.maxDocumentLength = maxDocLength
	return b
}

func (b *scrapeBuilder) SetUserAgent(s string) ScrapeBuilder {
	b.scrapeSettings.userAgent = s
	return b
}

func NewScrapeBuilder() ScrapeBuilder {
	return &scrapeBuilder{
		scrapeSettings: scrapeSettings{userAgent: "GoScraper"},
	}
}

type ScraperOptions struct {
	MaxDocumentLength int64
	UserAgent         string
}

type Scraper struct {
	Url                *url.URL
	EscapedFragmentUrl *url.URL
	MaxRedirect        int
	Options            ScraperOptions
}

type Document struct {
	Body      bytes.Buffer
	Preview   DocumentPreview
	ResHeader ResHeaders
}

type ResHeaders struct {
	ContentType string
}

type DocumentPreview struct {
	Icon        string
	Name        string
	Title       string
	Description string
	Type        string
	Images      []string
	Link        string
}

type ScrapeService interface {
	Scrape() (*Document, error)
	GetDocument() (*Document, error)
	ParseDocument(doc *Document) (*Document, error)
}

func Scrape(uri string, maxRedirect int, options ScraperOptions) (*Document, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	return (&Scraper{Url: u, MaxRedirect: maxRedirect, Options: options}).Scrape()
}

func (scraper *Scraper) Scrape() (*Document, error) {
	doc, err := scraper.getDocument()
	if err != nil {
		return nil, err
	}
	err = scraper.parseDocument(doc)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (scraper *Scraper) GetDocument() (*Document, error) {
	return scraper.getDocument()
}

func (scraper *Scraper) ParseDocument(doc *Document) (*Document, error) {
	err := scraper.parseDocument(doc)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (scraper *Scraper) getUrl() string {
	if scraper.EscapedFragmentUrl != nil {
		return scraper.EscapedFragmentUrl.String()
	}
	return scraper.Url.String()
}

func (scraper *Scraper) toFragmentUrl() error {
	unescapedurl, err := url.QueryUnescape(scraper.Url.String())
	if err != nil {
		return err
	}
	matches := fragmentRegexp.FindStringSubmatch(unescapedurl)
	if len(matches) > 1 {
		escapedFragment := EscapedFragment
		for _, r := range matches[1] {
			b := byte(r)
			if avoidByte(b) {
				continue
			}
			if escapeByte(b) {
				escapedFragment += url.QueryEscape(string(r))
			} else {
				escapedFragment += string(r)
			}
		}

		p := "?"
		if len(scraper.Url.Query()) > 0 {
			p = "&"
		}
		fragmentUrl, err := url.Parse(strings.Replace(unescapedurl, matches[0], p+escapedFragment, 1))
		if err != nil {
			return err
		}
		scraper.EscapedFragmentUrl = fragmentUrl
	} else {
		p := "?"
		if len(scraper.Url.Query()) > 0 {
			p = "&"
		}
		fragmentUrl, err := url.Parse(unescapedurl + p + EscapedFragment)
		if err != nil {
			return err
		}
		scraper.EscapedFragmentUrl = fragmentUrl
	}
	return nil
}

func (scraper *Scraper) getDocument() (*Document, error) {
	addUserAgent := func(req *http.Request) *http.Request {
		userAgent := "GoScraper"
		if len(scraper.Options.UserAgent) != 0 {
			userAgent = scraper.Options.UserAgent
		}
		req.Header.Add("User-Agent", userAgent)

		return req
	}

	scraper.MaxRedirect -= 1
	if strings.Contains(scraper.Url.String(), "#!") {
		scraper.toFragmentUrl()
	}
	if strings.Contains(scraper.Url.String(), EscapedFragment) {
		scraper.EscapedFragmentUrl = scraper.Url
	}

	if scraper.Options.MaxDocumentLength > 0 {
		// We try first to check content length (if it's present) - and if isn't - already limit by body size
		req, err := http.NewRequest("HEAD", scraper.getUrl(), nil)
		if err == nil {
			req = addUserAgent(req)

			resp, err := http.DefaultClient.Do(req)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err == nil {
				if resp.ContentLength > scraper.Options.MaxDocumentLength {
					return nil, errors.New("Content-Length exceed limits")
				}
			}
		}
	}

	req, err := http.NewRequest("GET", scraper.getUrl(), nil)
	if err != nil {
		return nil, err
	}
	req = addUserAgent(req)

	resp, err := http.DefaultClient.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	if resp.Request.URL.String() != scraper.getUrl() {
		scraper.EscapedFragmentUrl = nil
		scraper.Url = resp.Request.URL
	}

	if scraper.Options.MaxDocumentLength > 0 {
		resp.Body = http.MaxBytesReader(nil, resp.Body, scraper.Options.MaxDocumentLength)
	}

	b, err := convertUTF8(resp.Body, resp.Header.Get("content-type"))
	if err != nil {
		return nil, err
	}
	doc := &Document{
		Body:      b,
		Preview:   DocumentPreview{Link: scraper.Url.String()},
		ResHeader: ResHeaders{ContentType: resp.Header.Get("content-type")},
	}

	return doc, nil
}

func convertUTF8(content io.Reader, contentType string) (bytes.Buffer, error) {
	buff := bytes.Buffer{}
	content, err := charset.NewReader(content, contentType)
	if err != nil {
		return buff, err
	}
	_, err = io.Copy(&buff, content)
	if err != nil {
		return buff, err
	}
	return buff, nil
}

func (scraper *Scraper) parseDocument(doc *Document) error {
	t := html.NewTokenizer(&doc.Body)
	var ogImage bool
	var headPassed bool
	var hasFragment bool
	var hasCanonical bool
	var canonicalUrl *url.URL
	doc.Preview.Images = []string{}
	// saves previews' link in case that <link rel="canonical"> is found after <meta property="og:url">
	link := doc.Preview.Link
	// set default value to site name if <meta property="og:site_name"> not found
	doc.Preview.Name = scraper.Url.Host
	// set default icon to web root if <link rel="icon" href="/favicon.ico"> not found
	doc.Preview.Icon = fmt.Sprintf("%s://%s%s", scraper.Url.Scheme, scraper.Url.Host, "/favicon.ico")
	for {
		tokenType := t.Next()
		if tokenType == html.ErrorToken {
			return nil
		}
		if tokenType != html.SelfClosingTagToken && tokenType != html.StartTagToken && tokenType != html.EndTagToken {
			continue
		}
		token := t.Token()

		switch token.Data {
		case "head":
			if tokenType == html.EndTagToken {
				headPassed = true
			}
		case "body":
			headPassed = true

		case "link":
			var canonical bool
			var hasIcon bool
			var href string
			for _, attr := range token.Attr {
				if cleanStr(attr.Key) == "rel" && cleanStr(attr.Val) == "canonical" {
					canonical = true
				}
				if cleanStr(attr.Key) == "rel" && strings.Contains(cleanStr(attr.Val), "icon") {
					hasIcon = true
				}
				if cleanStr(attr.Key) == "href" {
					href = attr.Val
				}
				if len(href) > 0 && canonical && link != href {
					hasCanonical = true
					var err error
					canonicalUrl, err = url.Parse(href)
					if err != nil {
						return err
					}
				}
				if len(href) > 0 && hasIcon {
					doc.Preview.Icon = href
				}
			}

		case "meta":
			if len(token.Attr) != 2 {
				break
			}
			if metaFragment(token) && scraper.EscapedFragmentUrl == nil {
				hasFragment = true
			}
			var property string
			var content string
			for _, attr := range token.Attr {
				if cleanStr(attr.Key) == "property" || cleanStr(attr.Key) == "name" {
					property = attr.Val
				}
				if cleanStr(attr.Key) == "content" {
					content = attr.Val
				}
			}
			switch cleanStr(property) {
			case "og:site_name":
				doc.Preview.Name = content
			case "og:title":
				doc.Preview.Title = content
			case "og:type":
				doc.Preview.Type = content
			case "og:description":
				doc.Preview.Description = content
			case "description":
				if len(doc.Preview.Description) == 0 {
					doc.Preview.Description = content
				}
			case "og:url":
				doc.Preview.Link = content
			case "og:image":
				ogImage = true
				ogImgUrl, err := url.Parse(content)
				if err != nil {
					return err
				}
				if !ogImgUrl.IsAbs() {
					ogImgUrl.Host = scraper.Url.Host
					ogImgUrl.Scheme = scraper.Url.Scheme
				}

				doc.Preview.Images = []string{ogImgUrl.String()}

			}

		case "title":
			if tokenType == html.StartTagToken {
				t.Next()
				token = t.Token()
				if len(doc.Preview.Title) == 0 {
					doc.Preview.Title = token.Data
				}
			}

		case "img":
			for _, attr := range token.Attr {
				if cleanStr(attr.Key) == "src" {
					imgUrl, err := url.Parse(attr.Val)
					if err != nil {
						return err
					}
					if !imgUrl.IsAbs() {
						if string(imgUrl.Path[0]) == "/" {
							doc.Preview.Images = append(doc.Preview.Images, fmt.Sprintf("%s://%s%s", scraper.Url.Scheme, scraper.Url.Host, imgUrl.Path))
						} else {
							doc.Preview.Images = append(doc.Preview.Images, fmt.Sprintf("%s://%s/%s", scraper.Url.Scheme, scraper.Url.Host, imgUrl.Path))
						}
					} else {
						doc.Preview.Images = append(doc.Preview.Images, attr.Val)
					}

				}
			}
		}

		if hasCanonical && headPassed && scraper.MaxRedirect > 0 {
			if !canonicalUrl.IsAbs() {
				absCanonical, err := url.Parse(fmt.Sprintf("%s://%s%s", scraper.Url.Scheme, scraper.Url.Host, canonicalUrl.Path))
				if err != nil {
					return err
				}
				canonicalUrl = absCanonical
			}
			scraper.Url = canonicalUrl
			scraper.EscapedFragmentUrl = nil
			fdoc, err := scraper.getDocument()
			if err != nil {
				return err
			}
			*doc = *fdoc
			return scraper.parseDocument(doc)
		}

		if hasFragment && headPassed && scraper.MaxRedirect > 0 {
			scraper.toFragmentUrl()
			fdoc, err := scraper.getDocument()
			if err != nil {
				return err
			}
			*doc = *fdoc
			return scraper.parseDocument(doc)
		}

		if len(doc.Preview.Title) > 0 && len(doc.Preview.Description) > 0 && ogImage && headPassed {
			return nil
		}

	}

	return nil
}

func avoidByte(b byte) bool {
	i := int(b)
	if i == 127 || (i >= 0 && i <= 31) {
		return true
	}
	return false
}

func escapeByte(b byte) bool {
	i := int(b)
	if i == 32 || i == 35 || i == 37 || i == 38 || i == 43 || (i >= 127 && i <= 255) {
		return true
	}
	return false
}

func metaFragment(token html.Token) bool {
	var name string
	var content string

	for _, attr := range token.Attr {
		if cleanStr(attr.Key) == "name" {
			name = attr.Val
		}
		if cleanStr(attr.Key) == "content" {
			content = attr.Val
		}
	}
	if name == "fragment" && content == "!" {
		return true
	}
	return false
}

func cleanStr(str string) string {
	return strings.ToLower(strings.TrimSpace(str))
}
