package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/html"
)

var (
	// Storage directory for uploaded html files
	storageDir = "storage/"
	// Serve directory for static html
	serveDir = "static/"
)

// Serve index.html
func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, serveDir+"index.html")
}

// Receive .html file data via POST
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// Parse the form data
	r.ParseMultipartForm(10 << 20) // 10MB

	// Get a reference to the file headers
	file, handler, err := r.FormFile("file")
	if err != nil {
		fmt.Println("Error Retrieving the File")
		fmt.Println(err)
		return
	}
	defer file.Close()

	// Create the storage directory if it doesn't exist
	if _, err := os.Stat(storageDir); os.IsNotExist(err) {
		os.Mkdir(storageDir, 0755)
	}

	// Create the file on the server
	f, err := os.OpenFile(storageDir+handler.Filename, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Write the file to the server
	defer f.Close()
	if _, err := io.Copy(f, file); err != nil {
		fmt.Println(err)
		return
	}

	results, steamCmdStr, runStr, err := parsePreset(storageDir + handler.Filename)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Failed to parse! Error: " + err.Error()))
	}

	// respond with the result as marshalled json
	// Marshal the response
	response := ResultResponse{Results: results, SteamCmd: steamCmdStr, Run: runStr}
	responseBytes, err := json.Marshal(response)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Failed to marshal! Error: " + err.Error()))
	}

	// Write the response
	w.Write(responseBytes)

}

type ResultResponse struct {
	Results  []PresetResult `json:"results"`
	SteamCmd string         `json:"steamcmd"`
	Run      string         `json:"run"`
}

type PresetResult struct {
	ModName string `json:"modName"`
	ModID   string `json:"modID"`
}

func parsePreset(path string) (
	results []PresetResult,
	steamCmdStr string,
	runStr string,
	err error,
) {

	var cmdResult []string

	// Parse the preset file
	preset, err := os.Open(path)
	if err != nil {
		return []PresetResult{}, "", "", err
	}

	defer preset.Close()

	// Parse the html
	z, err := html.Parse(preset)
	if err != nil {
		return []PresetResult{}, "", "", err
	}

	// Get any <a> elements, select the href attribute, and if the link starts with https://steamcommunity.com/sharedfiles/filedetails/?id= then grab the id

	var f func(*html.Node)
	var getNameAndID func(*html.Node, *PresetResult)

	getNameAndID = func(n *html.Node, thisMod *PresetResult) {

		for c := n.FirstChild; c != nil; c = c.NextSibling {

			// Check if the node is a <td> element
			if !(c.Type == html.ElementNode && c.Data == "td") {
				continue
			}

			for _, a := range c.Attr {
				if a.Key == "data-type" {
					if a.Val == "DisplayName" {
						// Get the mod name
						thisMod.ModName = c.FirstChild.Data
						// fmt.Println(thisMod.ModName)
					}
				}
			}

			// If we found a <td>, we should look for an <a>
			for first := c.FirstChild; first != nil; first = first.NextSibling {
				if first.Type == html.ElementNode && first.Data == "a" {
					for _, a := range first.Attr {
						if a.Key == "href" {
							if strings.Contains(a.Val, "steamcommunity.com/sharedfiles/filedetails/?id=") {
								// Get the mod id
								thisMod.ModID = a.Val[55:]
								// fmt.Println(thisMod.ModID)
							}
						}
					}
				}
			}
		}
	}

	f = func(n *html.Node) {

		// Check if the node is a <tr> element with data-type="ModContainer"
		if n.Type == html.ElementNode && n.Data == "tr" {
			for _, a := range n.Attr {
				if a.Key == "data-type" && a.Val == "ModContainer" {
					// parse the children and fill presetResult fields
					thisResult := &PresetResult{}
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						getNameAndID(n, thisResult)
					}
					// if thisResult.ModID != "" {
					results = append(results, *thisResult)
					cmdResult = append(cmdResult, thisResult.ModID)
					runStr += fmt.Sprintf("@/steamcmd/steamapps/workshop/content/%s;", thisResult.ModID)
					// }
				}
			}
		}

		// Recurse
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}

	f(z)

	fmt.Printf("Found %d mods\n", len(results))

	// Build the command line
	for _, id := range cmdResult {
		steamCmdStr += "+workshop_download_item 107410 " + id + " "
	}

	// Build the run command
	runStr = fmt.Sprintf(`mod="%s"`, runStr)

	return results, steamCmdStr, runStr, nil
}

func main() {
	// Set up the handlers
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/upload", uploadHandler)

	// Start the server
	fmt.Println("Server listening on port 8080")
	http.ListenAndServe(":8080", nil)
}
