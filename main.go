package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const baseDir = "2_pali"
const paliAnalysisURL = "https://dpdict.net/"

// FileInfo represents a file or directory in the tree
type FileInfo struct {
	Name     string
	Path     string
	IsDir    bool
	Children []*FileInfo
}

// PageData holds data for template rendering
type PageData struct {
	Title       string
	Content     template.HTML
	Files       *FileInfo
	CurrentPath string
	Breadcrumbs []Breadcrumb
}

// Breadcrumb for navigation
type Breadcrumb struct {
	Name string
	Path string
}

var templates *template.Template

func main() {
	var err error
	templates, err = template.New("").Funcs(template.FuncMap{
		"isLastIndex": func(index, length int) bool {
			return index == length-1
		},
	}).Parse(templatesHTML)
	if err != nil {
		log.Fatal("Error parsing templates:", err)
	}

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/read/", handleRead)
	http.HandleFunc("/static/style.css", handleCSS)

	port := "8000"
	fmt.Printf("Pali Reader starting on http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css")
	w.Write([]byte(cssContent))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	files := buildFileTree(baseDir, "")

	data := PageData{
		Title: "Pali Reader",
		Files: files,
	}

	err := templates.ExecuteTemplate(w, "index", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleRead(w http.ResponseWriter, r *http.Request) {
	filePath := strings.TrimPrefix(r.URL.Path, "/read/")
	if filePath == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	fullPath := filepath.Join(baseDir, filePath)

	// Security check - prevent directory traversal
	absBase, _ := filepath.Abs(baseDir)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absBase) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	if info.IsDir() {
		// Show directory listing
		files := buildFileTree(fullPath, filePath)
		breadcrumbs := buildBreadcrumbs(filePath)

		data := PageData{
			Title:       filepath.Base(filePath),
			Files:       files,
			CurrentPath: filePath,
			Breadcrumbs: breadcrumbs,
		}

		err := templates.ExecuteTemplate(w, "directory", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Read and process file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		http.Error(w, "Cannot read file", http.StatusInternalServerError)
		return
	}

	processedContent := processHTMContent(string(content))
	breadcrumbs := buildBreadcrumbs(filePath)

	// Extract title from filename
	title := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))

	data := PageData{
		Title:       title,
		Content:     template.HTML(processedContent),
		CurrentPath: filePath,
		Breadcrumbs: breadcrumbs,
	}

	err = templates.ExecuteTemplate(w, "reader", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func buildFileTree(dirPath, relativePath string) *FileInfo {
	root := &FileInfo{
		Name:  filepath.Base(dirPath),
		Path:  relativePath,
		IsDir: true,
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return root
	}

	// Separate directories and files
	var dirs, files []*FileInfo

	for _, entry := range entries {
		childPath := filepath.Join(relativePath, entry.Name())
		child := &FileInfo{
			Name:  entry.Name(),
			Path:  childPath,
			IsDir: entry.IsDir(),
		}

		if entry.IsDir() {
			dirs = append(dirs, child)
		} else if strings.HasSuffix(strings.ToLower(entry.Name()), ".htm") {
			files = append(files, child)
		}
	}

	// Sort alphabetically
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Name < dirs[j].Name
	})
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	// Directories first, then files
	root.Children = append(dirs, files...)

	return root
}

func buildBreadcrumbs(path string) []Breadcrumb {
	if path == "" {
		return nil
	}

	var breadcrumbs []Breadcrumb
	parts := strings.Split(path, string(filepath.Separator))
	currentPath := ""

	for _, part := range parts {
		if part == "" {
			continue
		}
		if currentPath == "" {
			currentPath = part
		} else {
			currentPath = filepath.Join(currentPath, part)
		}
		breadcrumbs = append(breadcrumbs, Breadcrumb{
			Name: part,
			Path: currentPath,
		})
	}

	return breadcrumbs
}

// processHTMContent processes the HTML content and makes Pali words clickable
func processHTMContent(content string) string {
	// Extract body content if present
	bodyStart := strings.Index(strings.ToLower(content), "<body")
	bodyEnd := strings.LastIndex(strings.ToLower(content), "</body>")

	if bodyStart != -1 {
		// Find the end of the opening body tag
		bodyTagEnd := strings.Index(content[bodyStart:], ">")
		if bodyTagEnd != -1 {
			bodyStart = bodyStart + bodyTagEnd + 1
		}
	} else {
		bodyStart = 0
	}

	if bodyEnd == -1 {
		bodyEnd = len(content)
	}

	bodyContent := content[bodyStart:bodyEnd]

	// Process the content to make words clickable
	return makeWordsClickable(bodyContent)
}

// isPaliChar checks if a rune is a valid Pali character
func isPaliChar(r rune) bool {
	// Basic Latin letters
	if unicode.IsLetter(r) {
		return true
	}
	// Allow combining diacritics commonly used in Pali
	if unicode.Is(unicode.Mn, r) { // Nonspacing marks
		return true
	}
	return false
}

// isWordChar checks if a rune should be part of a word
func isWordChar(r rune) bool {
	return isPaliChar(r) || r == '\'' || r == '\u2019'
}

// makeWordsClickable wraps each Pali word in an anchor tag
func makeWordsClickable(content string) string {
	var result strings.Builder

	// Regex to match HTML tags
	tagPattern := regexp.MustCompile(`<[^>]+>`)
	// Regex to match reference patterns like [PTS Page 001]
	refPattern := regexp.MustCompile(`\[[^\]]+\]`)

	// Split content into segments (tags and text)
	lastEnd := 0
	tagMatches := tagPattern.FindAllStringIndex(content, -1)

	if len(tagMatches) == 0 {
		return processTextSegment(content, refPattern)
	}

	for _, match := range tagMatches {
		// Process text before this tag
		if match[0] > lastEnd {
			textSegment := content[lastEnd:match[0]]
			result.WriteString(processTextSegment(textSegment, refPattern))
		}
		// Keep the tag as-is
		result.WriteString(content[match[0]:match[1]])
		lastEnd = match[1]
	}

	// Process remaining text after last tag
	if lastEnd < len(content) {
		result.WriteString(processTextSegment(content[lastEnd:], refPattern))
	}

	return result.String()
}

// processTextSegment processes a text segment (not inside HTML tags)
func processTextSegment(text string, refPattern *regexp.Regexp) string {
	var result strings.Builder

	// Find all reference patterns and process around them
	refMatches := refPattern.FindAllStringIndex(text, -1)

	if len(refMatches) == 0 {
		return processWords(text)
	}

	lastEnd := 0
	for _, match := range refMatches {
		// Process text before this reference
		if match[0] > lastEnd {
			result.WriteString(processWords(text[lastEnd:match[0]]))
		}
		// Keep the reference as-is (with styling)
		ref := text[match[0]:match[1]]
		result.WriteString(`<span class="reference">`)
		result.WriteString(template.HTMLEscapeString(ref))
		result.WriteString(`</span>`)
		lastEnd = match[1]
	}

	// Process remaining text
	if lastEnd < len(text) {
		result.WriteString(processWords(text[lastEnd:]))
	}

	return result.String()
}

// processWords splits text into words and makes them clickable
func processWords(text string) string {
	var result strings.Builder
	runes := []rune(text)
	i := 0

	for i < len(runes) {
		// Check if this is the start of a word
		if isWordChar(runes[i]) {
			// Collect the entire word
			wordStart := i
			for i < len(runes) && isWordChar(runes[i]) {
				i++
			}
			word := string(runes[wordStart:i])

			// Clean word for URL (remove quotes, normalize)
			cleanWord := strings.ToLower(word)
			cleanWord = strings.Trim(cleanWord, "''\"")

			if len(cleanWord) > 0 && containsLetter(cleanWord) {
				// Create clickable link
				linkURL := fmt.Sprintf("%s?tab=dpd&q=%s",
					paliAnalysisURL, url.QueryEscape(cleanWord))
				fmt.Fprintf(&result, `<a href="%s" class="pali-word" target="other">%s</a>`,
					linkURL, template.HTMLEscapeString(word))
			} else {
				result.WriteString(template.HTMLEscapeString(word))
			}
		} else {
			// Non-word character - keep as is
			result.WriteRune(runes[i])
			i++
		}
	}

	return result.String()
}

// containsLetter checks if a string contains at least one letter
func containsLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

const templatesHTML = `
{{define "base"}}
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - Pali Reader</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <header>
        <div class="header-content">
            <a href="/" class="logo">
                <span class="logo-icon">☸</span>
                <span class="logo-text">Pali Reader</span>
            </a>
            <nav class="breadcrumbs">
                <a href="/">Home</a>
                {{range $i, $bc := .Breadcrumbs}}
                <span class="separator">›</span>
                {{if isLastIndex $i (len $.Breadcrumbs)}}
                <span class="current">{{$bc.Name}}</span>
                {{else}}
                <a href="/read/{{$bc.Path}}">{{$bc.Name}}</a>
                {{end}}
                {{end}}
            </nav>
        </div>
    </header>
    <main>
        {{template "content" .}}
    </main>
    <footer>
        <p>Click any Pali word to view its analysis on <a href="https://pali.sirimangalo.org" target="_blank">pali.sirimangalo.org</a></p>
    </footer>
</body>
</html>
{{end}}

{{define "index"}}
{{template "base" .}}
{{end}}

{{define "directory"}}
{{template "base" .}}
{{end}}

{{define "reader"}}
{{template "base" .}}
{{end}}

{{define "content"}}
<div class="container">
    {{if .Content}}
    <article class="reader-content">
        <h1>{{.Title}}</h1>
        <div class="pali-text">
            {{.Content}}
        </div>
    </article>
    {{else}}
    <div class="file-browser">
        <h1>{{if .CurrentPath}}{{.Title}}{{else}}Pali Texts Library{{end}}</h1>
        <p class="intro">Browse the collection of Pali texts. Click on any folder to explore, or select a text to read.</p>

        {{if .Files}}
        <div class="file-grid">
            {{range .Files.Children}}
            <a href="/read/{{.Path}}" class="file-card {{if .IsDir}}folder{{else}}file{{end}}">
                <div class="file-icon">
                    {{if .IsDir}}📁{{else}}📜{{end}}
                </div>
                <div class="file-name">{{.Name}}</div>
            </a>
            {{end}}
        </div>
        {{end}}
    </div>
    {{end}}
</div>
{{end}}
`

const cssContent = `
/* CSS Variables for theming */
:root {
    --primary-color: #8B4513;
    --primary-light: #D2691E;
    --primary-dark: #5D2E0C;
    --secondary-color: #F5DEB3;
    --background-color: #FDF5E6;
    --text-color: #333;
    --text-light: #666;
    --border-color: #DEB887;
    --card-shadow: 0 2px 8px rgba(139, 69, 19, 0.15);
    --link-color: #8B4513;
    --link-hover: #D2691E;
    --font-pali: 'Noto Sans', 'Noto Serif', 'Gentium Plus', 'Gentium', Georgia, serif;
}

/* Reset and base styles */
* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

html {
    scroll-behavior: smooth;
}

body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
    background-color: var(--background-color);
    color: var(--text-color);
    line-height: 1.6;
    min-height: 100vh;
    display: flex;
    flex-direction: column;
}

/* Header */
header {
    background: linear-gradient(135deg, var(--primary-color), var(--primary-dark));
    color: white;
    padding: 1rem 2rem;
    position: sticky;
    top: 0;
    z-index: 100;
    box-shadow: 0 2px 10px rgba(0,0,0,0.2);
}

.header-content {
    max-width: 1400px;
    margin: 0 auto;
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 1rem;
}

.logo {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    text-decoration: none;
    color: white;
}

.logo-icon {
    font-size: 2rem;
}

.logo-text {
    font-size: 1.5rem;
    font-weight: 600;
    letter-spacing: 0.5px;
}

/* Breadcrumbs */
.breadcrumbs {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    font-size: 0.9rem;
}

.breadcrumbs a {
    color: rgba(255,255,255,0.85);
    text-decoration: none;
    padding: 0.25rem 0.5rem;
    border-radius: 4px;
    transition: all 0.2s;
}

.breadcrumbs a:hover {
    background: rgba(255,255,255,0.15);
    color: white;
}

.breadcrumbs .separator {
    color: rgba(255,255,255,0.5);
}

.breadcrumbs .current {
    color: var(--secondary-color);
    font-weight: 500;
}

/* Main content */
main {
    flex: 1;
    padding: 2rem;
}

.container {
    max-width: 1200px;
    margin: 0 auto;
}

/* File browser */
.file-browser h1 {
    color: var(--primary-dark);
    margin-bottom: 0.5rem;
    font-size: 2rem;
}

.intro {
    color: var(--text-light);
    margin-bottom: 2rem;
    font-size: 1.1rem;
}

.file-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 1.5rem;
}

.file-card {
    background: white;
    border: 1px solid var(--border-color);
    border-radius: 12px;
    padding: 1.5rem;
    text-decoration: none;
    color: var(--text-color);
    transition: all 0.3s ease;
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    box-shadow: var(--card-shadow);
}

.file-card:hover {
    transform: translateY(-4px);
    box-shadow: 0 8px 24px rgba(139, 69, 19, 0.2);
    border-color: var(--primary-light);
}

.file-card.folder:hover {
    background: linear-gradient(135deg, #FFF8DC, white);
}

.file-card.file:hover {
    background: linear-gradient(135deg, #F5F5DC, white);
}

.file-icon {
    font-size: 3rem;
    margin-bottom: 0.75rem;
}

.file-name {
    font-weight: 500;
    word-break: break-word;
    font-size: 0.95rem;
}

/* Reader content */
.reader-content {
    background: white;
    border-radius: 16px;
    padding: 3rem;
    box-shadow: var(--card-shadow);
    border: 1px solid var(--border-color);
}

.reader-content h1 {
    color: var(--primary-dark);
    margin-bottom: 2rem;
    padding-bottom: 1rem;
    border-bottom: 2px solid var(--secondary-color);
    font-size: 2rem;
}

.pali-text {
    font-family: var(--font-pali);
    font-size: 1.2rem;
    line-height: 2;
    color: var(--text-color);
}

.pali-text br + br {
    display: block;
    content: "";
    margin-top: 1em;
}

/* Pali word links */
.pali-word {
    color: var(--link-color);
    text-decoration: none;
    border-bottom: 1px dotted var(--border-color);
    padding: 0 2px;
    border-radius: 2px;
    transition: all 0.2s ease;
}

.pali-word:hover {
    background-color: var(--secondary-color);
    color: var(--primary-dark);
    border-bottom-color: var(--primary-color);
}

/* Reference markers */
.reference {
    display: inline-block;
    background: #E8E0D5;
    color: var(--text-light);
    font-size: 0.75rem;
    padding: 0.15rem 0.4rem;
    border-radius: 4px;
    margin: 0 0.25rem;
    font-family: monospace;
    vertical-align: middle;
}

/* Horizontal rules */
.pali-text hr {
    border: none;
    height: 1px;
    background: linear-gradient(to right, transparent, var(--border-color), transparent);
    margin: 2rem 0;
}

/* Tables in content */
.pali-text table {
    margin: 1.5rem 0;
    border-collapse: collapse;
    font-size: 0.9rem;
}

.pali-text td {
    padding: 0.5rem 1rem;
    border: 1px solid var(--border-color);
}

/* Footer */
footer {
    background: var(--primary-dark);
    color: rgba(255,255,255,0.8);
    text-align: center;
    padding: 1.5rem 2rem;
    margin-top: auto;
}

footer a {
    color: var(--secondary-color);
    text-decoration: none;
}

footer a:hover {
    text-decoration: underline;
}

/* Responsive adjustments */
@media (max-width: 768px) {
    header {
        padding: 1rem;
    }

    .header-content {
        flex-direction: column;
        align-items: flex-start;
    }

    main {
        padding: 1rem;
    }

    .reader-content {
        padding: 1.5rem;
    }

    .pali-text {
        font-size: 1.1rem;
        line-height: 1.8;
    }

    .file-grid {
        grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
        gap: 1rem;
    }

    .file-card {
        padding: 1rem;
    }

    .file-icon {
        font-size: 2.5rem;
    }
}

/* Print styles */
@media print {
    header, footer {
        display: none;
    }

    .reader-content {
        box-shadow: none;
        border: none;
        padding: 0;
    }

    .pali-word {
        color: black;
        border-bottom: none;
    }
}
`
