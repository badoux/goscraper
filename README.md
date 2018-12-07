# goscraper
[Golang](http://golang.org/) package to quickly return a preview of a webpage, you can get easily its title, description & images

## Usage
	goscraper.Scrape(<url>, <max redirects>, <timeout in milliseconds>, <useragent>)

    func main() {
		s, err := goscraper.Scrape("https://www.w3.org/", 5, 2000, "Mozilla/5.0 (X11; Linux i686; rv:64.0) Gecko/20100101 Firefox/64.0")
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("Icon : %s\n", s.Preview.Icon)
		fmt.Printf("Name : %s\n", s.Preview.Name)
		fmt.Printf("Title : %s\n", s.Preview.Title)
		fmt.Printf("Description : %s\n", s.Preview.Description)
		fmt.Printf("Image: %s\n", s.Preview.Images[0])
		fmt.Printf("Url : %s\n", s.Preview.Link)
	}

output:

**Icon :** https://www.w3.org/favicon.ico  
**Name :** www.w3.org  
**Title :** World Wide Web Consortium (W3C)  
**Description :** The World Wide Web Consortium (W3C) is an international community where Member organizations, a full-time staff, and the public work together to develop Web standards.  
**Image:** https://www.w3.org/2008/site/images/logo-w3c-mobile-lg  
**Url :** https://www.w3.org/


## License

Goscraper is licensed under the [MIT License](./LICENSE).
