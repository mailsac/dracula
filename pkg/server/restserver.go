package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

type BaseResponse struct {
	Message string `json:"message"`
	Details string `json:"details"`
}

type CountResponse struct {
	Count int `json:"count"`
}

type ListResponse struct {
	List []string `json:"list"`
}

func MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
	resp := BaseResponse{Message: "Method not allowed", Details: r.Method + " " + r.URL.Path}
	json.NewEncoder(w).Encode(resp)
}

func NotMatchedHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	resp := BaseResponse{Message: "Route not matched", Details: r.Method + " " + r.URL.Path}
	json.NewEncoder(w).Encode(resp)
}

func GetBaseHandler(s *Server, w http.ResponseWriter, r *http.Request) {
	resp := BaseResponse{Message: "OK", Details: "Dracula rest server - Routes:  GET /namespaces, GET /count, GET /put"}
	json.NewEncoder(w).Encode(resp)
}

func CountHandler(s *Server, w http.ResponseWriter, r *http.Request) {
	queryParams := r.URL.Query()
	namespace := strings.Trim(queryParams.Get("namespace"), " \n")
	pattern := strings.Trim(queryParams.Get("pattern"), " \n")
	if namespace == "" || pattern == "" {
		w.WriteHeader(http.StatusBadRequest)
		resp := BaseResponse{Message: "Bad request", Details: "namespace and pattern query params are required"}
		json.NewEncoder(w).Encode(resp)
		return
	}
	count := s.store.Count(namespace, pattern)
	resp := CountResponse{Count: count}
	json.NewEncoder(w).Encode(resp)
}

func PutHandler(s *Server, w http.ResponseWriter, r *http.Request) {
	queryParams := r.URL.Query()
	namespace := strings.Trim(queryParams.Get("namespace"), " \n")
	key := strings.Trim(queryParams.Get("key"), " \n")
	if namespace == "" || key == "" {
		w.WriteHeader(http.StatusBadRequest)
		resp := BaseResponse{Message: "Bad request", Details: "namespace and key query params are required"}
		json.NewEncoder(w).Encode(resp)
		return
	}
	s.store.Put(namespace, key)
	count := s.store.Count(namespace, key)
	resp := CountResponse{Count: count}
	json.NewEncoder(w).Encode(resp)
}

func NamespacesHandler(s *Server, w http.ResponseWriter, r *http.Request) {
	namespaces := s.store.Namespaces()
	resp := ListResponse{List: namespaces}
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) restServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.log.Println(r.Method, r.URL.Path, r.URL.Query(), r.RemoteAddr)

	switch r.URL.Path {
	case "/":
		switch r.Method {
		case http.MethodGet:
			GetBaseHandler(s, w, r)
		default:
			MethodNotAllowedHandler(w, r)
		}
	case "/count":
		switch r.Method {
		case http.MethodGet:
			CountHandler(s, w, r)
		default:
			MethodNotAllowedHandler(w, r)
		}
	case "/put":
		switch r.Method {
		case http.MethodGet:
			PutHandler(s, w, r)
		default:
			MethodNotAllowedHandler(w, r)
		}
	case "/namespaces":
		switch r.Method {
		case http.MethodGet:
			NamespacesHandler(s, w, r)
		default:
			MethodNotAllowedHandler(w, r)
		}
	default:
		NotMatchedHandler(w, r)
	}
}
