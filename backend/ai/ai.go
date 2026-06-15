package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"backend/db"
	"backend/storage"
	"backend/ws"
)

var ollamaURL string

func init() {
	url := os.Getenv("OLLAMA_HOST")
	if url == "" {
		url = "http://localhost:11434"
	}
	ollamaURL = url
}

// ExtractTextFromImage invokes Tesseract OCR on a local file path and returns the text
func ExtractTextFromImage(imagePath string) (string, error) {
	// Execute tesseract: tesseract <imagePath> stdout
	cmd := exec.Command("tesseract", imagePath, "stdout")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("tesseract execution failed: %w (stderr: %s)", err, stderr.String())
	}

	return strings.TrimSpace(out.String()), nil
}

type OllamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type OllamaGenerateResponse struct {
	Response string `json:"response"`
}

// CallOllama calls the local Ollama REST API with a prompt
func CallOllama(ctx context.Context, prompt string) (string, error) {
	// Use small fast models like llama3:8b or qwen2:1.5b
	model := os.Getenv("OLLAMA_MODEL")
	if model == "" {
		model = "qwen2:1.5b" // Default low-RAM friendly model
	}

	reqBody := OllamaGenerateRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", ollamaURL+"/api/generate", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed calling Ollama service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned non-200 status %d: %s", resp.StatusCode, string(respBytes))
	}

	var genResp OllamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return "", err
	}

	return strings.TrimSpace(genResp.Response), nil
}

// GenerateSummary prompts Ollama to summarize text in 3 lines
func GenerateSummary(ctx context.Context, text string) (string, error) {
	// Cap text length to avoid context window overhead
	if len(text) > 4000 {
		text = text[:4000]
	}
	prompt := fmt.Sprintf("Summarize the following document text in 3 clear bullet points. Do not include introductory remarks, output only the bullet points:\n\n%s", text)
	return CallOllama(ctx, prompt)
}

// GenerateTags prompts Ollama to suggest tags for a file name and content
func GenerateTags(ctx context.Context, filename, text string) ([]string, error) {
	if len(text) > 1500 {
		text = text[:1500]
	}
	prompt := fmt.Sprintf("Based on the file name '%s' and this sample text snippet, list exactly 3 to 5 simple, single-word tags that describe the file. Return only the tags as a comma-separated list without quotes, spaces, or sentences. Example output: invoice,finance,june:\n\n%s", filename, text)
	tagsStr, err := CallOllama(ctx, prompt)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(tagsStr, ",")
	var tags []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			tags = append(tags, strings.ToLower(t))
		}
	}
	return tags, nil
}

// ProcessFileAIBackground orchestrates OCR, Summarization, and Tagging on a background worker thread
func ProcessFileAIBackground(fileID string, filename string, mimeType string, objectKey string) {
	go func() {
		// Wait brief moment to allow upload transaction to lock and commit completely
		time.Sleep(1 * time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		log.Printf("Background AI Job Started: file %s (%s)", filename, fileID)

		var extractedText string
		var summary string
		var tags []string

		// 1. Check if Image & Run OCR
		isImage := strings.HasPrefix(mimeType, "image/")
		if isImage {
			// Download temp file to local disk for tesseract cli
			tempFile, err := os.CreateTemp("", "cloudstore_ocr_*.tmp")
			if err == nil {
				defer os.Remove(tempFile.Name())
				defer tempFile.Close()

				// Download S3 Object reader
				objectReader, downloadErr := storage.GetObject(ctx, objectKey)
				if downloadErr == nil {
					_, _ = io.Copy(tempFile, objectReader)
					objectReader.Close()

					// Run OCR
					ocrText, ocrErr := ExtractTextFromImage(tempFile.Name())
					if ocrErr == nil && ocrText != "" {
						extractedText = ocrText
						log.Printf("AI OCR text extracted successfully from %s (%d chars)", filename, len(ocrText))
					} else if ocrErr != nil {
						log.Printf("AI OCR Error for %s: %v", filename, ocrErr)
					}
				}
			}
		}

		// 2. Perform Summarization & Tagging if text content exists (either from OCR or text file upload)
		
		// Note: For actual PDF parsing in Go, a package like pdfcpu would be used.
		// For simplicity, if it's text/plain we read it. If pdf, we summarize based on file name or metadata.
		textSample := extractedText
		if textSample == "" && strings.HasPrefix(mimeType, "text/") {
			objectReader, downloadErr := storage.GetObject(ctx, objectKey)
			if downloadErr == nil {
				buf := new(bytes.Buffer)
				_, _ = io.Copy(buf, objectReader)
				objectReader.Close()
				textSample = buf.String()
			}
		}

		if textSample == "" {
			// Fallback text if binary/uncaught
			textSample = "Document uploaded named: " + filename
		}

		// Ollama LLM requests
		aiSummary, sumErr := GenerateSummary(ctx, textSample)
		if sumErr == nil {
			summary = aiSummary
		} else {
			log.Printf("AI summary failed for %s: %v", filename, sumErr)
		}

		aiTags, tagErr := GenerateTags(ctx, filename, textSample)
		if tagErr == nil {
			tags = aiTags
		} else {
			log.Printf("AI tagging failed for %s: %v", filename, tagErr)
		}

		// 3. Save to database
		_, err := db.Pool.Exec(ctx, `
			UPDATE files 
			SET extracted_text = $1, summary = $2, tags = $3, updated_at = NOW() 
			WHERE id = $4`, 
			extractedText, summary, tags, fileID)
		if err != nil {
			log.Printf("Failed to save AI results to DB: %v", err)
			return
		}

		// 4. Retrieve User Owner ID
		var userID string
		_ = db.Pool.QueryRow(ctx, "SELECT user_id FROM files WHERE id = $1", fileID).Scan(&userID)

		// 5. Broadcast real-time completion message via WebSocket!
		msgPayload, _ := json.Marshal(map[string]interface{}{
			"type":              "notification",
			"title":             "AI Processing Complete",
			"message":           fmt.Sprintf("AI has finished analyzing '%s'. Summary and tags are ready!", filename),
			"notification_type": "success",
			"file_id":           fileID,
		})
		ws.BroadcastToUser(userID, msgPayload)

		log.Printf("Background AI Job Finished: file %s", filename)
	}()
}
