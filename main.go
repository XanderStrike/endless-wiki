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
	r.HandleFunc("/stream/{article}", streamHandler).Methods("GET")
	
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
	
	// Render the streaming page template
	renderStreamingWikiPage(w, articleName)
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	articleName := vars["article"]
	
	if articleName == "" {
		http.Error(w, "Article name is required", http.StatusBadRequest)
		return
	}
	
	// Set headers for Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	// Generate article content using Ollama with streaming
	err := generateArticleStream(articleName, w)
	if err != nil {
		log.Printf("Error generating article: %v", err)
		fmt.Fprintf(w, "event: error\ndata: Failed to generate article\n\n")
	}
	
	// Send completion event
	fmt.Fprintf(w, "event: complete\ndata: done\n\n")
}

func generateArticleStream(articleName string, w http.ResponseWriter) error {
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}
	
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "llama2"
	}
	
	log.Printf("Generating article '%s' using model '%s' at host '%s'", articleName, ollamaModel, ollamaHost)
	
	prompt := fmt.Sprintf(`You are a wiki article generator. Generate a comprehensive, informative article about "%s" in markdown format. 

Requirements:
- Write in an encyclopedic style with extensive cross-referencing
- Include multiple sections with appropriate headers
- Add TONS of links to related topics using markdown link syntax [Topic Name](topic-name)
- Link every relevant concept, person, place, technology, theory, or related topic mentioned
- Aim for 20-50+ links throughout the article - the more the better!
- Link both obvious connections and tangential related topics
- Make the article detailed and informative with rich interconnections
- Use proper markdown formatting
- Think of this as creating a web of knowledge where readers can explore endlessly

IMPORTANT: Be very generous with links. If you mention any concept that could be its own article, link it! Examples:
- Historical periods, events, people
- Scientific concepts, theories, discoveries
- Geographic locations, countries, cities
- Technologies, inventions, methods
- Cultural movements, philosophies, religions
- Academic fields, disciplines, subjects
- Organizations, institutions, companies

Generate the article now:`, articleName)
	
	reqBody := OllamaRequest{
		Model:  ollamaModel,
		Prompt: prompt,
		Stream: true,
	}
	
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	
	resp, err := http.Post(ollamaHost+"/api/generate", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	decoder := json.NewDecoder(resp.Body)
	var fullContent strings.Builder
	
	for {
		var ollamaResp OllamaResponse
		if err := decoder.Decode(&ollamaResp); err != nil {
			break
		}
		
		if ollamaResp.Response != "" {
			fullContent.WriteString(ollamaResp.Response)
			
			// Convert the accumulated markdown to HTML
			htmlContent := blackfriday.Run([]byte(fullContent.String()))
			processedContent := processWikiLinks(string(htmlContent))
			
			// Send the updated content via SSE
			fmt.Fprintf(w, "event: content\ndata: %s\n\n", strings.ReplaceAll(processedContent, "\n", "\\n"))
			
			// Flush the response
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		
		if ollamaResp.Done {
			break
		}
	}
	
	return nil
}

func processWikiLinks(content string) string {
	// Convert markdown links to wiki links
	// This is a simple regex replacement - in production you might want more sophisticated parsing
	content = strings.ReplaceAll(content, `<a href="`, `<a href="/wiki/`)
	return content
}

func renderStreamingWikiPage(w http.ResponseWriter, title string) {
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
        .loading { 
            color: #666; 
            font-style: italic; 
        }
        .loading::after {
            content: '';
            animation: dots 1.5s steps(5, end) infinite;
        }
        @keyframes dots {
            0%, 20% { content: ''; }
            40% { content: '.'; }
            60% { content: '..'; }
            80%, 100% { content: '...'; }
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
    
    <div class="content" id="content">
        <div class="loading">Generating article</div>
    </div>
    
    <script>
        const eventSource = new EventSource('/stream/{{.Title}}');
        const contentDiv = document.getElementById('content');
        
        eventSource.onmessage = function(event) {
            // Handle default messages
        };
        
        eventSource.addEventListener('content', function(event) {
            const content = event.data.replace(/\\n/g, '\n');
            contentDiv.innerHTML = content;
        });
        
        eventSource.addEventListener('complete', function(event) {
            eventSource.close();
        });
        
        eventSource.addEventListener('error', function(event) {
            contentDiv.innerHTML = '<p style="color: red;">Error generating article. Please try again.</p>';
            eventSource.close();
        });
        
        eventSource.onerror = function(event) {
            contentDiv.innerHTML = '<p style="color: red;">Connection error. Please try again.</p>';
            eventSource.close();
        };
    </script>
</body>
</html>`
	
	t, err := template.New("wiki").Parse(tmpl)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
	
	data := struct {
		Title string
	}{
		Title: title,
	}
	
	w.Header().Set("Content-Type", "text/html")
	if err := t.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}
