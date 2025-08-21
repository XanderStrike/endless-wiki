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
	
	prompt := fmt.Sprintf(`You are a wiki article generator. Generate a comprehensive, informative article about "%s" in plain text format (no markdown). 

Requirements:
- Write in an encyclopedic style
- Include multiple sections with clear section headers
- Make the article detailed and informative
- Use proper paragraph structure
- Do NOT use any markdown formatting - just plain text
- Focus on creating quality content about the topic

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
			
			// Convert plain text to HTML with every word as a link
			htmlContent := makeEveryWordClickable(fullContent.String())
			
			// Send the updated content via SSE
			fmt.Fprintf(w, "event: content\ndata: %s\n\n", strings.ReplaceAll(htmlContent, "\n", "\\n"))
			
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

func makeEveryWordClickable(content string) string {
	// Split content into lines to preserve structure
	lines := strings.Split(content, "\n")
	var result strings.Builder
	
	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}
		
		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			result.WriteString("<br>")
			continue
		}
		
		// Check if this looks like a header (simple heuristic)
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 0 && (strings.HasSuffix(trimmed, ":") || 
			(len(trimmed) < 100 && !strings.Contains(trimmed, ".") && 
			 strings.ToUpper(trimmed[:1]) == trimmed[:1])) {
			result.WriteString("<h3>")
			result.WriteString(makeWordsClickable(line))
			result.WriteString("</h3>")
		} else {
			result.WriteString("<p>")
			result.WriteString(makeWordsClickable(line))
			result.WriteString("</p>")
		}
	}
	
	return result.String()
}

func makeWordsClickable(text string) string {
	// Split text into words while preserving punctuation
	words := strings.Fields(text)
	var result strings.Builder
	
	for i, word := range words {
		if i > 0 {
			result.WriteString(" ")
		}
		
		// Extract the actual word from punctuation
		cleanWord := strings.Trim(word, ".,!?;:()[]{}\"'")
		
		// Skip very short words and common words that don't make good articles
		if len(cleanWord) <= 2 || isCommonWord(cleanWord) {
			result.WriteString(word)
		} else {
			// Get the prefix and suffix punctuation
			prefix := word[:len(word)-len(strings.TrimLeft(word, ".,!?;:()[]{}\"'"))]
			suffix := word[len(strings.TrimRight(word, ".,!?;:()[]{}\"'")):]
			
			result.WriteString(prefix)
			result.WriteString(fmt.Sprintf(`<a href="/wiki/%s">%s</a>`, cleanWord, cleanWord))
			result.WriteString(suffix)
		}
	}
	
	return result.String()
}

func isCommonWord(word string) bool {
	commonWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "have": true, "has": true, "had": true, "do": true,
		"does": true, "did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "can": true, "must": true, "shall": true,
		"this": true, "that": true, "these": true, "those": true, "it": true, "its": true,
		"he": true, "she": true, "they": true, "we": true, "you": true, "i": true,
		"me": true, "him": true, "her": true, "them": true, "us": true, "my": true,
		"your": true, "his": true, "her": true, "their": true, "our": true,
		"as": true, "so": true, "if": true, "when": true, "where": true, "why": true,
		"how": true, "what": true, "who": true, "which": true, "than": true, "then": true,
		"now": true, "here": true, "there": true, "up": true, "down": true, "out": true,
		"off": true, "over": true, "under": true, "again": true, "further": true,
		"once": true, "more": true, "most": true, "other": true, "some": true, "any": true,
		"each": true, "few": true, "all": true, "both": true, "either": true, "neither": true,
		"not": true, "no": true, "nor": true, "too": true, "very": true, "just": true,
		"only": true, "own": true, "same": true, "such": true, "into": true, "from": true,
		"about": true, "after": true, "before": true, "during": true, "between": true,
		"through": true, "above": true, "below": true, "because": true, "until": true,
		"while": true, "since": true, "although": true, "though": true, "unless": true,
	}
	
	return commonWords[strings.ToLower(word)]
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
