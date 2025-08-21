package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
	"github.com/russross/blackfriday/v2"
)

type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type OllamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func main() {
	r := mux.NewRouter()
	
	r.HandleFunc("/", homeHandler).Methods("GET")
	r.HandleFunc("/wiki/{article}", wikiHandler).Methods("GET")
	
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	
	log.Printf("Starting endless wiki server on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>Endless Wiki</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        h1 { color: #333; }
        .search-box { margin: 20px 0; }
        input[type="text"] { padding: 10px; width: 300px; font-size: 16px; }
        button { padding: 10px 20px; font-size: 16px; background: #007cba; color: white; border: none; cursor: pointer; }
        button:hover { background: #005a87; }
        .examples { margin-top: 30px; }
        .examples a { display: block; margin: 5px 0; color: #007cba; text-decoration: none; }
        .examples a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <h1>Welcome to Endless Wiki</h1>
    <p>An infinite wiki powered by AI. Search for any topic and get a generated article with links to explore further.</p>
    
    <div class="search-box">
        <input type="text" id="searchInput" placeholder="Enter any topic..." onkeypress="handleKeyPress(event)">
        <button onclick="searchWiki()">Generate Article</button>
    </div>
    
    <div class="examples">
        <h3>Try these examples:</h3>
        <a href="/wiki/Quantum Computing">Quantum Computing</a>
        <a href="/wiki/Ancient Rome">Ancient Rome</a>
        <a href="/wiki/Machine Learning">Machine Learning</a>
        <a href="/wiki/Space Exploration">Space Exploration</a>
        <a href="/wiki/Renaissance Art">Renaissance Art</a>
    </div>
    
    <script>
        function handleKeyPress(event) {
            if (event.key === 'Enter') {
                searchWiki();
            }
        }
        
        function searchWiki() {
            const input = document.getElementById('searchInput');
            const topic = input.value.trim();
            if (topic) {
                window.location.href = '/wiki/' + encodeURIComponent(topic);
            }
        }
    </script>
</body>
</html>`
	
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func wikiHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	articleName := vars["article"]
	
	if articleName == "" {
		http.Error(w, "Article name is required", http.StatusBadRequest)
		return
	}
	
	// Generate article content using Ollama
	content, err := generateArticle(articleName)
	if err != nil {
		log.Printf("Error generating article: %v", err)
		http.Error(w, "Failed to generate article", http.StatusInternalServerError)
		return
	}
	
	// Convert markdown to HTML
	htmlContent := blackfriday.Run([]byte(content))
	
	// Process links to point to our wiki
	processedContent := processWikiLinks(string(htmlContent))
	
	// Render the page
	renderWikiPage(w, articleName, processedContent)
}

func generateArticle(articleName string) (string, error) {
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}
	
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "llama2"
	}
	
	prompt := fmt.Sprintf(`You are a wiki article generator. Generate a comprehensive, informative article about "%s" in markdown format. 

Requirements:
- Write in an encyclopedic style
- Include multiple sections with appropriate headers
- Add links to related topics using markdown link syntax [Topic Name](topic-name)
- Make the article detailed and informative
- Include at least 5-10 links to related articles
- Use proper markdown formatting

Generate the article now:`, articleName)
	
	reqBody := OllamaRequest{
		Model:  ollamaModel,
		Prompt: prompt,
		Stream: false,
	}
	
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	
	resp, err := http.Post(ollamaHost+"/api/generate", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", err
	}
	
	return ollamaResp.Response, nil
}

func processWikiLinks(content string) string {
	// Convert markdown links to wiki links
	// This is a simple regex replacement - in production you might want more sophisticated parsing
	content = strings.ReplaceAll(content, `<a href="`, `<a href="/wiki/`)
	return content
}

func renderWikiPage(w http.ResponseWriter, title, content string) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>{{.Title}} - Endless Wiki</title>
    <style>
        body { 
            font-family: Georgia, serif; 
            max-width: 900px; 
            margin: 0 auto; 
            padding: 20px; 
            line-height: 1.6;
        }
        .header { 
            border-bottom: 1px solid #ccc; 
            margin-bottom: 20px; 
            padding-bottom: 10px;
        }
        .header h1 { 
            margin: 0; 
            color: #333; 
        }
        .nav { 
            margin-bottom: 20px; 
        }
        .nav a { 
            color: #007cba; 
            text-decoration: none; 
            margin-right: 15px;
        }
        .nav a:hover { 
            text-decoration: underline; 
        }
        .content { 
            font-size: 16px; 
        }
        .content h1, .content h2, .content h3 { 
            color: #333; 
            border-bottom: 1px solid #eee; 
            padding-bottom: 5px;
        }
        .content a { 
            color: #007cba; 
            text-decoration: none; 
        }
        .content a:hover { 
            text-decoration: underline; 
        }
        .content p { 
            margin-bottom: 15px; 
        }
        .content ul, .content ol { 
            margin-bottom: 15px; 
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>{{.Title}}</h1>
    </div>
    
    <div class="nav">
        <a href="/">← Home</a>
        <a href="javascript:history.back()">← Back</a>
    </div>
    
    <div class="content">
        {{.Content}}
    </div>
</body>
</html>`
	
	t, err := template.New("wiki").Parse(tmpl)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
	
	data := struct {
		Title   string
		Content template.HTML
	}{
		Title:   title,
		Content: template.HTML(content),
	}
	
	w.Header().Set("Content-Type", "text/html")
	if err := t.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}
