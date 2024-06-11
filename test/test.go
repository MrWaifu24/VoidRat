package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func mockPrint(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	var data map[string]interface{}
	json.Unmarshal(b, &data)
	fmt.Println(data)
}

func main() {
	http.HandleFunc("/newListener", mockPrint)
	http.HandleFunc("/runCommand", mockPrint)
	http.HandleFunc("/getZombies", mockPrint)
	http.HandleFunc("/getListeners", mockPrint)
	http.HandleFunc("/stopListener", mockPrint)
	http.HandleFunc("/removeZombie", mockPrint)

	err := http.ListenAndServe(":2560", nil)
	if err != nil {
		fmt.Println(err)
		return
	}
}
