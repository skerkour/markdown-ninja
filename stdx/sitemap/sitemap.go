package sitemap

import (
	"bytes"
	"encoding/xml"
	"io"
	"time"
)

// https://www.sitemaps.org/protocol.html

// ChangeFreq specifies change frequency of a sitemap entry. It is just a string.
type ChangeFreq string

const (
	Always  ChangeFreq = "always"
	Hourly  ChangeFreq = "hourly"
	Daily   ChangeFreq = "daily"
	Weekly  ChangeFreq = "weekly"
	Monthly ChangeFreq = "monthly"
	Yearly  ChangeFreq = "yearly"
	Never   ChangeFreq = "never"
)

type Sitemap struct {
	XMLName xml.Name `xml:"urlset"`
	Xmlns   string   `xml:"xmlns,attr"`
	Urls    []URL    `xml:"url"`
	minify  bool     `xml:"-"`
}

type URL struct {
	Loc        string     `xml:"loc"`
	LastMod    *time.Time `xml:"lastmod,omitempty"`
	ChangeFreq ChangeFreq `xml:"changefreq,omitempty"`
	Priority   float32    `xml:"priority,omitempty"`
}

// New returns a new Sitemap.
func New(minify bool) *Sitemap {
	return &Sitemap{
		Xmlns:  "http://www.sitemaps.org/schemas/sitemap/0.9",
		Urls:   make([]URL, 0),
		minify: minify,
	}
}

// Add adds an URL to a Sitemap.
func (sitemap *Sitemap) Add(url URL) {
	sitemap.Urls = append(sitemap.Urls, url)
}

// WriteTo writes XML encoded sitemap to given io.Writer.
// Implements io.WriterTo.
func (sitemap *Sitemap) WriteTo(w io.Writer) (err error) {
	_, err = w.Write([]byte(xml.Header))
	if err != nil {
		return
	}

	xmlEncoder := xml.NewEncoder(w)
	if !sitemap.minify {
		xmlEncoder.Indent("", "  ")
	}

	err = xmlEncoder.Encode(sitemap)
	w.Write([]byte{'\n'})
	return
}

func (sitemap *Sitemap) String() (ret string, err error) {
	buffer := bytes.NewBufferString("")

	err = sitemap.WriteTo(buffer)
	if err != nil {
		return
	}

	ret = buffer.String()
	return
}
