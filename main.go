package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const defaultPort = "8081"
const museumDir = "./web/html/museu"
const adminUser = "adm"
const adminPass = "wsderfgtt"
const sessionCookieName = "museum_session"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	fileServer := http.FileServer(http.Dir(museumDir))
	mux := http.NewServeMux()

	// Página principal e arquivos estáticos
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/amuseu.html", http.StatusTemporaryRedirect)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	// Rota para a página de administração
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(museumDir, "admin.html"))
	})

	// Rota para obter as obras
	mux.HandleFunc("GET /obras", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(museumDir, "obras.json"))
	})

	// Rota para atualizar uma obra
	mux.HandleFunc("POST /update-obra", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var updatedObra struct {
			ID      int    `json:"id"`
			Titulo  string `json:"titulo"`
			Autor   string `json:"autor"`
			Tecnica string `json:"tecnica"`
		}

		if err := json.NewDecoder(r.Body).Decode(&updatedObra); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Ler o arquivo atual
		jsonPath := filepath.Join(museumDir, "obras.json")
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			http.Error(w, "Error reading file", http.StatusInternalServerError)
			return
		}

		var obras []struct {
			ID      int    `json:"id"`
			Titulo  string `json:"titulo"`
			Autor   string `json:"autor"`
			Tecnica string `json:"tecnica"`
		}

		if err := json.Unmarshal(data, &obras); err != nil {
			http.Error(w, "Error parsing JSON", http.StatusInternalServerError)
			return
		}

		// Atualizar a obra correspondente
		found := false
		for i := range obras {
			if obras[i].ID == updatedObra.ID {
				obras[i].Titulo = updatedObra.Titulo
				obras[i].Autor = updatedObra.Autor
				obras[i].Tecnica = updatedObra.Tecnica
				found = true
				break
			}
		}

		if !found {
			http.Error(w, "Obra not found", http.StatusNotFound)
			return
		}

		// Salvar de volta
		newData, err := json.MarshalIndent(obras, "", "  ")
		if err != nil {
			http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
			return
		}

		if err := os.WriteFile(jsonPath, newData, 0644); err != nil {
			http.Error(w, "Error writing file", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	// Rota para reordenar obras
	mux.HandleFunc("POST /reorder-obras", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var payload struct {
			IDs []int `json:"ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		if len(payload.IDs) != 33 {
			http.Error(w, "Exactly 33 IDs required", http.StatusBadRequest)
			return
		}

		// Ler obras atuais
		jsonPath := filepath.Join(museumDir, "obras.json")
		data, err := os.ReadFile(jsonPath)
		if err != nil {
			http.Error(w, "Error reading file", http.StatusInternalServerError)
			return
		}

		type Obra struct {
			ID      int    `json:"id"`
			Titulo  string `json:"titulo"`
			Autor   string `json:"autor"`
			Tecnica string `json:"tecnica"`
		}

		var currentObras []Obra
		if err := json.Unmarshal(data, &currentObras); err != nil {
			http.Error(w, "Error parsing JSON", http.StatusInternalServerError)
			return
		}

		// Criar mapa para acesso rápido
		obrasMap := make(map[int]Obra)
		for _, o := range currentObras {
			obrasMap[o.ID] = o
		}

		// Nova lista de obras mantendo os IDs fixos 1..33
		// mas pegando os dados da nova ordem de IDs
		newObras := make([]Obra, 33)

		// Para renomear imagens, vamos usar um diretório temporário
		tempDir, err := os.MkdirTemp("", "museum_reorder")
		if err != nil {
			http.Error(w, "Error creating temp dir", http.StatusInternalServerError)
			return
		}
		defer os.RemoveAll(tempDir)

		for i, oldID := range payload.IDs {
			newID := i + 1
			obraOriginal := obrasMap[oldID]

			newObras[i] = Obra{
				ID:      newID,
				Titulo:  obraOriginal.Titulo,
				Autor:   obraOriginal.Autor,
				Tecnica: obraOriginal.Tecnica,
			}

			// Mover imagem para temp com o novo nome
			oldImgPath := filepath.Join(museumDir, fmt.Sprintf("tela%d.jpg", oldID))
			newImgPathTemp := filepath.Join(tempDir, fmt.Sprintf("tela%d.jpg", newID))

			// Se a imagem existir, copia para o temp
			if _, err := os.Stat(oldImgPath); err == nil {
				input, _ := os.ReadFile(oldImgPath)
				os.WriteFile(newImgPathTemp, input, 0644)
			}
		}

		// Sobrescrever imagens originais a partir do temp
		for i := 1; i <= 33; i++ {
			tempImgPath := filepath.Join(tempDir, fmt.Sprintf("tela%d.jpg", i))
			finalImgPath := filepath.Join(museumDir, fmt.Sprintf("tela%d.jpg", i))

			if _, err := os.Stat(tempImgPath); err == nil {
				input, _ := os.ReadFile(tempImgPath)
				os.WriteFile(finalImgPath, input, 0644)
			}
		}

		// Salvar novo obras.json
		newData, err := json.MarshalIndent(newObras, "", "  ")
		if err != nil {
			http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
			return
		}

		if err := os.WriteFile(jsonPath, newData, 0644); err != nil {
			http.Error(w, "Error writing file", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	// Login
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		var creds struct {
			User string `json:"user"`
			Pass string `json:"pass"`
		}
		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		if creds.User == adminUser && creds.Pass == adminPass {
			http.SetCookie(w, &http.Cookie{
				Name:     sessionCookieName,
				Value:    "authenticated",
				Path:     "/",
				HttpOnly: true,
				Expires:  time.Now().Add(24 * time.Hour),
			})
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})

	// Verificar sessão
	mux.HandleFunc("GET /check-session", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Upload de imagem
	mux.HandleFunc("POST /upload", func(w http.ResponseWriter, r *http.Request) {
		if !isAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse multipart form
		if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB max
			http.Error(w, "File too large", http.StatusBadRequest)
			return
		}

		id := r.FormValue("id")
		file, _, err := r.FormFile("image")
		if err != nil {
			http.Error(w, "Error retrieving file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Validar ID (1 a 33)
		var idInt int
		if _, err := fmt.Sscanf(id, "%d", &idInt); err != nil || idInt < 1 || idInt > 33 {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		dstPath := filepath.Join(museumDir, fmt.Sprintf("tela%d.jpg", idInt))
		dst, err := os.Create(dstPath)
		if err != nil {
			http.Error(w, "Error saving file", http.StatusInternalServerError)
			return
		}
		defer dst.Close()

		if _, err := io.Copy(dst, file); err != nil {
			http.Error(w, "Error saving file", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	addr := ":" + port
	log.Printf("museunobre disponível em http://127.0.0.1:%s/amuseu.html", port)
	log.Printf("Administração disponível em http://127.0.0.1:%s/admin", port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return cookie.Value == "authenticated"
}
