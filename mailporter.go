package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/mail"

	"github.com/bgentry/speakeasy"
	"github.com/cheggaaa/pb"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/codegangsta/cli"
	"github.com/mxk/go-imap/imap"
)

const notes = "pbfb5a.uni-paderborn.de"
const exchange = "ex.uni-paderborn.de"
const tmpFile = "mailporter-credentials"
const maxSize = 50000000

type Login struct {
	User     string
	Password string
}

func getCredentials(server string) (string, string) {
	content, _ := ioutil.ReadFile(os.TempDir() + "/" + tmpFile)
	credentials := map[string]Login{}
	json.Unmarshal(content, &credentials)
	creds, ok := credentials[server]
	if !ok {
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("Benutzer für %s: ", server)
		user, _ := reader.ReadString('\n')
		user = strings.TrimSpace(user)
		password, _ := speakeasy.Ask(fmt.Sprintf("Passwort für %s (nicht angezeigt): ", server))
		password = strings.TrimSpace(password)
		credentials[server] = Login{User: user, Password: password}
		content, _ = json.Marshal(credentials)
		ioutil.WriteFile(os.TempDir()+"/"+tmpFile, content, 0600)
		return user, password
	}
	return creds.User, creds.Password
}

func imapConnect(server string) *imap.Client {
	user, password := getCredentials(server)
	c, err := imap.DialTLS(server, nil)
	if err != nil {
		log.Fatal(err)
	}
	if c.State() == imap.Login {
		if _, err := c.Login(user, password); err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatal("Connection not in Login state. Cannot login.")
	}
	return c
}

func transferMbox(c *cli.Context) {
	source := c.Args().Get(0)
	target := c.Args().Get(1)
	if source == "" || target == "" {
		log.Fatal("Bitte Quell- und Zielordner angeben, z.B.: mailporter transfer INBOX INBOX")
	}
	n := imapConnect(notes)
	defer n.Logout(30 * time.Second)
	_, err := n.Select(source, true)
	if err != nil {
		log.Fatal("Kein Zugriff auf %s/%s. Fehler: %s", notes, source, err.Error())
	}
	e := imapConnect(exchange)
	defer e.Logout(30 * time.Second)

	fmt.Println("Ermittle zu übertragende Mails...")
	criteria := []string{"ALL"}
	if c.String("before") != "" {
		criteria = append(criteria, "BEFORE "+c.String("before"))
	}
	nc, err := imap.Wait(n.UIDSearch(strings.Join(criteria, " ")))
	var mails []uint32
	for _, r := range nc.Data {
		mails = r.SearchResults()
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%d Mails sind zu übertragen. Fortfahren (j oder n)? ", len(mails))
	user, _ := reader.ReadString('\n')
	if user != "j\n" {
		return
	}
	fmt.Printf("Übertrage Mails.\n")
	bar := pb.StartNew(len(mails))
	set, _ := imap.NewSeqSet("")
	for _, mid := range mails {
		set.AddNum(mid)
	}
	fetch, err := n.UIDFetch(set, "BODY.PEEK[]")
	if err != nil {
		log.Fatalf("Konnte Mails nicht laden: ", err)
	}
	flags := map[string]bool{
		"\\Seen": true,
	}
	for fetch.InProgress() {
		n.Recv(-1)
		for _, r := range fetch.Data {
			i := r.MessageInfo()
			if i.Size >= maxSize {
				m, err := mail.ReadMessage(bytes.NewReader(imap.AsBytes(i.Attrs["BODY[]"])))
				if err != nil {
					log.Fatal(err)
				}
				date, _ := m.Header.Date()
				datestring := date.Format(time.RFC822)
				fmt.Printf("WARNUNG: Mail '%s' (%s, von %s) ist zu groß für Exchange. Überspringe.\n",
					m.Header.Get("Subject"), datestring, m.Header.Get("From"))
				fetch.Data = nil
				n.Data = nil
				continue
			}
			_, err := imap.Wait(e.Append(target, flags, nil,
				imap.NewLiteral(imap.AsBytes(i.Attrs["BODY[]"]))))
			if err != nil {
				fmt.Printf("WARNUNG: Konnte Mail nicht auf Exchange speichern.\nFehler: %s\n", err.Error())
				m, err := mail.ReadMessage(bytes.NewReader(imap.AsBytes(i.Attrs["BODY[]"])))
				if err != nil {
					log.Fatal(err)
				}
				date, _ := m.Header.Date()
				datestring := date.Format(time.RFC822)
				fmt.Println("Von: ", m.Header.Get("From"))
				fmt.Println("Betreff: ", m.Header.Get("Subject"))
				fmt.Println("Datum: ", datestring)
				e.Logout(0)
				e = imapConnect(exchange)
				defer e.Logout(30 * time.Second)
				fetch.Data = nil
				n.Data = nil
				continue
			}
			bar.Increment()
		}
		fetch.Data = nil
		n.Data = nil
	}
}

func listCommand(c *cli.Context) {
	var server string
	if c.Args().First() == "" || c.Args().First() == "notes" {
		server = notes
	} else {
		server = exchange
	}
	client := imapConnect(server)
	defer client.Logout(30 * time.Second)
	cmd, _ := imap.Wait(client.List("", "%"))
	fmt.Printf("Ordner auf %s:\n", server)
	for _, r := range cmd.Data {
		fmt.Printf("- %s\n", r.MailboxInfo().Name)
	}
}

func main() {
	app := cli.NewApp()
	app.Name = "mailporter"
	app.Usage = "Tool zum Umzug von Notes nach Exchange."
	app.Commands = []cli.Command{
		{
			Name:   "list",
			Usage:  "IMAP-Ordner auflisten",
			Action: listCommand,
		},
		{
			Name:   "transfer",
			Usage:  "Mails von Notes nach Exchange transferieren",
			Action: transferMbox,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "before, b",
					Usage: "Nur Mails vor diesem Datum übertragen, z.B. -b 19-Aug-2015.",
				},
			},
		},
		{
			Name:  "clear",
			Usage: "Zwischengespeicherte Exchange- und Notes-Logins löschen",
			Action: func(c *cli.Context) {
				os.Remove(os.TempDir() + "/" + tmpFile)
			},
		},
	}
	app.Run(os.Args)
}
