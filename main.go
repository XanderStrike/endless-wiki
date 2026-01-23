package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
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

var supportedLangs = map[string]struct {
	Name   string
	Prompt func(article string) string
}{
	"en": {
		Name: "English",
		Prompt: func(article string) string {
			return fmt.Sprintf(`You are a wiki article generator. Generate a comprehensive informative article about "%s" in markdown format.

Requirements:
- Write like wikipedia in an encyclopedic style
- Include multiple sections with clear markdown headers (## Section Name)
- Use proper markdown formatting including **bold**, *italic*, lists, etc.
- Include relevant subsections where appropriate
- Make the article detailed and informative
- Provide only the markdown text of the article, no followup questions

Generate the article now:`, article)
		},
	},
	"de": {
		Name: "Deutsch",
		Prompt: func(article string) string {
			return fmt.Sprintf(`Du bist ein Generator für Wiki-Artikel. Erstelle einen umfassenden, informativen Artikel über "%s" im Markdown-Format.

Anforderungen:
- Schreibe im enzyklopädischen Stil wie in der Wikipedia
- Verwende mehrere Abschnitte mit klaren Markdown-Überschriften (## Abschnittsname)
- Nutze korrektes Markdown mit **fett**, *kursiv*, Listen usw.
- Füge relevante Unterabschnitte hinzu, wo sinnvoll
- Der Artikel soll detailliert und informativ sein
- Gib nur den Markdown-Text des Artikels zurück, ohne Rückfragen

Erstelle den Artikel jetzt:`, article)
		},
	},
	"it": {
		Name: "Italiano",
		Prompt: func(article string) string {
			return fmt.Sprintf(`Sei un generatore di articoli wiki. Genera un articolo completo e informativo su "%s" in formato markdown.

Requisiti:
- Scrivi in stile enciclopedico come Wikipedia
- Includi più sezioni con intestazioni markdown chiare (## Nome Sezione)
- Usa una formattazione markdown corretta, inclusi **grassetto**, *corsivo*, elenchi, ecc.
- Includi sottosezioni rilevanti dove opportuno
- L'articolo deve essere dettagliato e informativo
- Fornisci solo il testo markdown dell'articolo, senza domande di follow-up

Genera ora l'articolo:`, article)
		},
	},
	"es": {
		Name: "Español",
		Prompt: func(article string) string {
			return fmt.Sprintf(`Eres un generador de artículos wiki. Genera un artículo completo e informativo sobre "%s" en formato markdown.

Requisitos:
- Escribe como Wikipedia en un estilo enciclopédico
- Incluye varias secciones con encabezados markdown claros (## Nombre de la Sección)
- Usa formato markdown adecuado incluyendo **negrita**, *cursiva*, listas, etc.
- Incluye subsecciones relevantes donde corresponda
- El artículo debe ser detallado e informativo
- Proporciona solo el texto markdown del artículo, sin preguntas de seguimiento

Genera el artículo ahora:`, article)
		},
	},
	"fr": {
		Name: "Français",
		Prompt: func(article string) string {
			return fmt.Sprintf(`Vous êtes un générateur d'articles wiki. Générez un article complet et informatif sur "%s" au format markdown.

Exigences :
- Écrivez dans un style encyclopédique comme Wikipédia
- Incluez plusieurs sections avec des titres markdown clairs (## Nom de la section)
- Utilisez une mise en forme markdown appropriée, y compris **gras**, *italique*, listes, etc.
- Ajoutez des sous-sections pertinentes si nécessaire
- L'article doit être détaillé et informatif
- Fournissez uniquement le texte markdown de l'article, sans questions de suivi

Générez l'article maintenant :`, article)
		},
	},
	"ja": {
		Name: "日本語",
		Prompt: func(article string) string {
			return fmt.Sprintf(`あなたはウィキ記事の生成者です。「%s」について、マークダウン形式で包括的かつ情報豊富な記事を作成してください。

要件:
- Wikipediaのような百科事典的な文体で執筆する
- 複数のセクションを明確なマークダウン見出し（## セクション名）で分ける
- **太字**、*斜体*、リストなど、適切なマークダウン書式を使用する
- 必要に応じて関連するサブセクションを含める
- 記事は詳細かつ情報豊富であること
- 記事のマークダウンテキストのみを返し、追加の質問はしない

今すぐ記事を生成してください:`, article)
		},
	},
	"zh": {
		Name: "中文",
		Prompt: func(article string) string {
			return fmt.Sprintf(`你是一个维基百科文章生成器。请用markdown格式生成一篇关于“%s”的全面且信息丰富的文章。

要求：
- 以百科全书式的风格（类似维基百科）撰写
- 包含多个部分，并使用清晰的markdown标题（## 部分名称）
- 正确使用markdown格式，包括**加粗**、*斜体*、列表等
- 适当时加入相关的子部分
- 文章应详细且信息丰富
- 只提供文章的markdown文本，不要附加后续问题

现在生成文章：`, article)
		},
	},
}

// getLang safely selects a language code, falling back to "en".
func getLang(r *http.Request) string {
	lang := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lang")))
	if _, ok := supportedLangs[lang]; ok {
		return lang
	}
	return "en"
}

func main() {
	// Ensure the preferred model is downloaded on startup
	ensureModelDownloaded()

	r := mux.NewRouter().UseEncodedPath()

	r.HandleFunc("/", homeHandler).Methods("GET")
	r.HandleFunc("/wiki/{article}", wikiHandler).Methods("GET")
	r.HandleFunc("/stream/{article}", streamHandler).Methods("GET")
	r.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/favicon.ico")
	}).Methods("GET")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))

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
	articleName, e := url.PathUnescape(vars["article"])

	if e != nil || articleName == "" {
		http.Error(w, "Article name is required", http.StatusBadRequest)
		return
	}

	lang := getLang(r)

	renderStreamingWikiPage(w, articleName, lang)
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	articleName, e := url.PathUnescape(vars["article"])
	if e != nil || articleName == "" {
		http.Error(w, "Article name is required", http.StatusBadRequest)
		return
	}

	// Get language from query; fallback handled inside getLang
	lang := getLang(r)

	// Set headers for Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a context that gets cancelled when the client disconnects
	ctx := r.Context()

	// Generate article content using Ollama with streaming
	err := generateArticleStream(ctx, articleName, lang, w)
	if err != nil {
		// Check if it was cancelled due to client disconnect
		if ctx.Err() == context.Canceled {
			log.Printf("Article generation cancelled for '%s' (client disconnected)", articleName)
			return
		}
		log.Printf("Error generating article: %v", err)
		fmt.Fprintf(w, "event: error\ndata: Failed to generate article\n\n")
		return
	}

	// Send completion event (only if not cancelled)
	if ctx.Err() == nil {
		fmt.Fprintf(w, "event: complete\ndata: done\n\n")
	}
}

func generateArticleStream(ctx context.Context, articleName, lang string, w http.ResponseWriter) error {
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}

	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "llama2"
	}

	// Select prompt by language; fallback already ensured by getLang
	promptTpl := supportedLangs[lang].Prompt
	prompt := promptTpl(articleName)

	log.Printf("Generating article '%s' using model '%s' at host '%s' in language '%s'", articleName, ollamaModel, ollamaHost, lang)

	reqBody := OllamaRequest{
		Model:  ollamaModel,
		Prompt: prompt,
		Stream: true,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	// Create HTTP request with context for cancellation
	req, err := http.NewRequestWithContext(ctx, "POST", ollamaHost+"/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var fullContent strings.Builder

	for {
		// Check if context was cancelled
		select {
		case <-ctx.Done():
			log.Printf("Article generation cancelled for '%s'", articleName)
			return ctx.Err()
		default:
		}

		var ollamaResp OllamaResponse
		if err := decoder.Decode(&ollamaResp); err != nil {
			// Check if it's a context cancellation error
			if ctx.Err() != nil {
				return ctx.Err()
			}
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

func ensureModelDownloaded() {
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}

	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "llama2"
	}

	log.Printf("Ensuring model '%s' is available at '%s'", ollamaModel, ollamaHost)

	// Try to pull the model
	pullReq := struct {
		Name string `json:"name"`
	}{
		Name: ollamaModel,
	}

	jsonData, err := json.Marshal(pullReq)
	if err != nil {
		log.Printf("Error marshaling pull request: %v", err)
		return
	}

	resp, err := http.Post(ollamaHost+"/api/pull", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Error pulling model (Ollama may not be ready yet): %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("Model '%s' is ready", ollamaModel)
	} else {
		log.Printf("Model pull returned status %d", resp.StatusCode)
	}
}

func renderStreamingWikiPage(w http.ResponseWriter, title, lang string) {
	tmpl, err := template.ParseFiles("templates/wiki.html")
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	data := struct {
		Title string
		Lang  string
	}{
		Title: title,
		Lang:  lang,
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}
