package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var sessionOutput map[string]chan []byte

func main() {
	if len(os.Args) > 1 {
		collector := make(chan string)
		var clients []chan string
		for _, host := range os.Args[1:] {
			ch := make(chan string)
			clients = append(clients, ch)
			fmt.Println("DEBUG: number of clients:", len(clients))
			fmt.Println("Connect to host", host)
			go connectToHost(host, ch)
			go func() {
				for s := range ch {
					fmt.Printf("Write %s to channel\n", s)
					collector <- s
					fmt.Printf("wrote %s to collector\n", s)
				}
			}()
		}
		for output := range collector {
			fmt.Println("DEBUG1")
			fmt.Println(output)
			fmt.Println("DEBUG2")
		}
	} else {
		fmt.Printf("Give the host names or IP addresses as command line option:\n"+
			"%s host1 host2 ...\n", os.Args[0])
	}
}

func connectToHost(host string, ch chan string) {
	conn, err := startSSHConnection(host)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		conn.Close()
		fmt.Println("connection closed")
	}()

	session, err := conn.NewSession()
	if err != nil {
		fmt.Printf("error:%s: Could not create SSH session", host)
		return
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		fmt.Printf("error:%s: Could not get PTY\n", host)
		return
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		fmt.Println("Could not redirect stdout to temporary file")
		return
	}
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := stdout.Read(buffer)
			if err != nil {
				if err == io.EOF {
					break
				}
				fmt.Println("error while reading remote stdout:", err)
				break
			}
			ch <- string(buffer[:n])
		}
		session.Close()
		fmt.Println("session closed")
	}()
	session.Run("hostname")
}

func createWindowForOutput(tmpFilename string) {
	cmd := exec.Command("/usr/bin/xterm", "-hold", "-e", "tail", "-f", tmpFilename)
	err := cmd.Run()
	fmt.Println("xterm:", err)
}

func startSSHConnection(host string) (*ssh.Client, error) {
	s := strings.Split(host, "@")
	var hostname string
	var user string
	switch len(s) {
	case 1:
		user = os.Getenv("USER")
		hostname = s[0]
	case 2:
		user = s[0]
		hostname = s[1]
	}

	var port string
	s = strings.Split(hostname, ":")
	switch len(s) {
	case 1:
		port = "22"
		hostname = s[0]
	case 2:
		hostname = s[0]
		port = s[1]
	}

	fmt.Println(user)
	fmt.Println(hostname)
	fmt.Println(port)
	sshConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{sshAgent()},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	hostPlusPort := fmt.Sprintf("%s:%s", hostname, port)
	fmt.Println("Try to connect to ", hostPlusPort)
	conn, err := ssh.Dial("tcp", hostPlusPort, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("Could not connect to %q:", hostPlusPort, err)
	}
	fmt.Println("Successful connected to ", hostPlusPort)
	return conn, nil
}

func sshAgent() ssh.AuthMethod {
	sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		log.Fatal(err)
	}
	return ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers)
}
