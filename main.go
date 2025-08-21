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
	tmpl, err := template.ParseFiles("templates/home.html")
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, nil); err != nil {
		log.Printf("Template execution error: %v", err)
	}
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

	prompt := fmt.Sprintf(`You are a wiki article generator. Generate a comprehensive informative article about "%s" in markdown format. 

Requirements:
- Write like wikipedia in an encyclopedic style
- Include multiple sections with clear markdown headers (## Section Name)
- Use proper markdown formatting including **bold**, *italic*, lists, etc.
- Include relevant subsections where appropriate
- Make the article detailed and informative
- Provide only the markdown text of the article, no followup questions

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

			// Send the raw markdown content via SSE (will be parsed by frontend)
			markdownContent := fullContent.String()
			fmt.Fprintf(w, "event: content\ndata: %s\n\n", strings.ReplaceAll(markdownContent, "\n", "\\n"))

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


func renderStreamingWikiPage(w http.ResponseWriter, title string) {
	tmpl, err := template.ParseFiles("templates/wiki.html")
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
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}
