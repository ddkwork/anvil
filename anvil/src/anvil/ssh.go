package main

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/ddkwork/golibrary/mylog"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var sshClientCache = NewSshClientCache(settings.Ssh.CacheSize)

// TODO: Support closing the connections after some delay.
// Also detect a network issue and re-open the connection.
type SshClientCache struct {
	data             map[SshEndpt]SshClientCacheEntry
	max              int
	lock             sync.Mutex
	keyfilePasswords map[string]string
	sshHopPasswords  map[SshHop]string
	keyfileAuths     []ssh.AuthMethod
	keys             map[string][]byte
}

func NewSshClientCache(max int) *SshClientCache {
	return &SshClientCache{
		data:             make(map[SshEndpt]SshClientCacheEntry),
		max:              max,
		keyfilePasswords: map[string]string{},
		sshHopPasswords:  map[SshHop]string{},
		keys:             map[string][]byte{},
	}
}

func (cache *SshClientCache) Get(endpt SshEndpt, kill chan struct{}) (client *SshClient, err error) {
	cache.lock.Lock()
	defer cache.lock.Unlock()
	defer func() {
		mylog.Check(prefixWithSshEndpt(endpt, "SshClientCache.Get", err))
	}()

	e, ok := cache.data[endpt]
	if !ok {
		client = mylog.Check2(cache.add(endpt, kill))
		return
	}

	if !cache.isValid(e.client.Client()) {
		// reconnect
		client = mylog.Check2(cache.add(endpt, kill))
		return
	}

	client = e.client

	return
}

func prefixWithSshEndpt(endpt SshEndpt, msg string, err error) error {
	if err == nil {
		return nil
	}
	if msg != "" {
		return fmt.Errorf("%s: %s: %w", endpt, msg, err)
	} else {
		return fmt.Errorf("%s: %w", endpt, err)
	}
}

func (cache *SshClientCache) isValid(client *ssh.Client) bool {
	// See https://datatracker.ietf.org/doc/html/draft-ssh-global-requests-ok-00 section 4.1 (active keepalive)
	status, payload := mylog.Check3(client.SendRequest("keep-alive@implementation.example.com", true, []byte("keep-alive")))
	log(LogCatgSsh, "cache.isValid: %v, %v, %v\n", status, payload, nil)

	return true
}

func (cache *SshClientCache) add(endpt SshEndpt, kill chan struct{}) (client *SshClient, err error) {
	if len(cache.data) >= cache.max {
		cache.rmLeastRecentlyUsed()
	}

	c := mylog.Check2(cache.dial(endpt, kill))

	client = &SshClient{client: c, endpt: endpt}

	cache.data[endpt] = SshClientCacheEntry{client: client, lastUsed: time.Now()}
	return
}

func (cache *SshClientCache) rmLeastRecentlyUsed() {
	var minK SshEndpt
	var minTime time.Time
	for k, v := range cache.data {
		if minTime.IsZero() || v.lastUsed.Before(minTime) {
			minTime = v.lastUsed
			minK = k
			continue
		}
	}

	delete(cache.data, minK)
}

func (cache *SshClientCache) dial(endpt SshEndpt, kill chan struct{}) (client *ssh.Client, err error) {
	log(LogCatgSsh, "SshClientCache: creating new ssh client object\n")

	dest := cache.completeHop(endpt.Dest)
	proxy := endpt.Proxy
	if endpt.HasProxy() {
		proxy = cache.completeHop(endpt.Proxy)
	}

	destAuths := cache.getAuths(dest)

	destConf := &ssh.ClientConfig{
		User:            dest.User,
		Auth:            destAuths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	proxyConf := (*ssh.ClientConfig)(nil)
	if endpt.HasProxy() {
		proxyAuths := cache.getAuths(proxy)
		proxyConf = &ssh.ClientConfig{
			User:            proxy.User,
			Auth:            proxyAuths,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
	}

	addr := fmt.Sprintf("%s:%s", dest.Host, dest.Port)
	proxyAddr := ""
	if endpt.HasProxy() {
		proxyAddr = fmt.Sprintf("%s:%s", proxy.Host, proxy.Port)
	}

	client = mylog.Check2(cache.dialOrKill("tcp", addr, destConf, proxyAddr, proxyConf, kill))
	return
}

func (cache *SshClientCache) completeHop(h SshHop) SshHop {
	if h.User == "" {
		if runtime.GOOS == "windows" {
			h.User = os.Getenv("USERNAME")
		} else {
			h.User = os.Getenv("USER")
		}
	}

	if h.Port == "" {
		h.Port = "22"
	}

	return h
}

func (cache *SshClientCache) dialOrKill(network, addr string, conf *ssh.ClientConfig, proxyAddr string, proxyConf *ssh.ClientConfig, kill chan struct{}) (client *ssh.Client, err error) {
	c := make(chan struct{})

	wakeup := func() {
		c <- struct{}{}
	}

	go func() {
		if proxyAddr != "" {
			client = mylog.Check2(cache.dialWithProxy(network, addr, conf, proxyAddr, proxyConf, kill))
		} else {
			client = mylog.Check2(ssh.Dial("tcp", addr, conf))
		}
		wakeup()
	}()

	select {
	case <-c:
		return
	case <-kill:
		mylog.
			// We just need to let the dial finish on it's own and we abandon the return values
			Check(fmt.Errorf("Dial to %s was killed", addr))
		return
	}
}

func (cache *SshClientCache) dialWithProxy(network, addr string, conf *ssh.ClientConfig, proxyAddr string, proxyConf *ssh.ClientConfig, kill chan struct{}) (client *ssh.Client, err error) {
	proxyClient := mylog.Check2(ssh.Dial("tcp", proxyAddr, proxyConf))

	conn := mylog.Check2(proxyClient.Dial(network, addr))

	ncc, chans, reqs := mylog.Check4(ssh.NewClientConn(conn, addr, conf))

	client = ssh.NewClient(ncc, chans, reqs)
	return
}

func (cache *SshClientCache) getAuths(hop SshHop) []ssh.AuthMethod {
	auths := cache.getKeyfileAuths()
	a := cache.getPasswordAuth(hop)
	if a != nil {
		r := make([]ssh.AuthMethod, len(auths)+1)
		copy(r, auths)
		r[len(auths)] = a
		auths = r
	}
	return auths
}

func (cache *SshClientCache) SetSshHopPassword(user, host, port, password string) {
	h := SshHop{User: user, Host: host, Port: port}
	h = cache.completeHop(h)
	cache.sshHopPasswords[h] = password
}

func (cache *SshClientCache) getPasswordAuth(hop SshHop) ssh.AuthMethod {
	pw, ok := cache.sshHopPasswords[hop]
	if !ok {
		return nil
	}
	log(LogCatgSsh, "Found password for ssh hop %v\n", hop)

	return ssh.Password(pw)
}

func (cache *SshClientCache) SetKeyfilePassword(filename, password string) {
	cache.keyfilePasswords[filename] = password
	cache.invalidateKeyfileAuths()
}

func (cache *SshClientCache) AddKeyFromFile(filename string, path string) {
	key := mylog.Check2(os.ReadFile(path))
	cache.keys[filename] = key
	cache.invalidateKeyfileAuths()
}

func (cache *SshClientCache) invalidateKeyfileAuths() {
	cache.keyfileAuths = nil
}

func (cache *SshClientCache) getKeyfileAuths() []ssh.AuthMethod {
	if cache.keyfileAuths == nil {
		cache.makeKeyfileAuths()
	}
	return cache.keyfileAuths
}

func (cache *SshClientCache) makeKeyfileAuths() {
	log(LogCatgSsh, "SshClientCache: building auths\n")

	signers, _ := cache.sshAgentSigners()

	for fname, key := range cache.keys {
		log(LogCatgSsh, "SshClientCache: making auth for key %s\n", fname)

		s := cache.signerForKey(fname, key)

		if s != nil {
			signers = append(signers, s)
		}
	}

	cache.keyfileAuths = []ssh.AuthMethod{ssh.PublicKeys(signers...)}
}

func (cache *SshClientCache) signerForKey(filename string, key []byte) ssh.Signer {
	pw, ok := cache.keyfilePasswords[filename]

	var s ssh.Signer

	if ok {
		log(LogCatgSsh, "Decoding key %s using password\n", filename)
		s = mylog.Check2(ssh.ParsePrivateKeyWithPassphrase(key, []byte(pw)))
	} else {
		log(LogCatgSsh, "Decoding key %s without password\n", filename)
		s = mylog.Check2(ssh.ParsePrivateKey(key))
	}

	return s
}

func (cache *SshClientCache) sshAgentSigners() ([]ssh.Signer, error) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	conn := mylog.Check2(net.Dial("unix", socket))

	log(LogCatgSsh, "Adding keys from ssh agent (SSH_AUTH_SOCK)\n")
	return agent.NewClient(conn).Signers()
}

func (cache *SshClientCache) Keys() []SshEndpt {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	keys := make([]SshEndpt, 0, len(cache.data))
	for k := range cache.data {
		keys = append(keys, k)
	}
	return keys
}

func (cache *SshClientCache) Entries() []SshClientCacheEntry {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	entries := make([]SshClientCacheEntry, 0, len(cache.data))
	for _, v := range cache.data {
		entries = append(entries, v)
	}
	return entries
}

type SshEndpt struct {
	Dest  SshHop
	Proxy SshHop
}

func (k SshEndpt) HasProxy() bool {
	return k.Proxy.Host != ""
}

func (k SshEndpt) String() string {
	if k.HasProxy() {
		return fmt.Sprintf("%s@%s:%s%%%s@%s:%s",
			k.Dest.User, k.Dest.Host, k.Dest.Port,
			k.Proxy.User, k.Proxy.Host, k.Proxy.Port,
		)
	} else {
		return fmt.Sprintf("%s@%s:%s", k.Dest.User, k.Dest.Host, k.Dest.Port)
	}
}

type SshHop struct {
	User, Host, Port string
}

type SshClientCacheEntry struct {
	client   *SshClient
	lastUsed time.Time
}

type SshClient struct {
	client       *ssh.Client
	endpt        SshEndpt
	listener     net.Listener
	listenerPort int
	userData     interface{}
}

func (s SshClient) Client() *ssh.Client {
	return s.client
}

func (s *SshClient) Listener() (net.Listener, error) {
	if s.listener != nil {
		return s.listener, nil
	}

	s.listener = mylog.Check2(s.client.Listen("tcp", "127.0.0.1:0"))

	tl, ok := s.listener.Addr().(*net.TCPAddr)
	if !ok {
		return s.listener, fmt.Errorf("SshClient.Listener: listener is not a *net.TCPAddr. Can't determine port.")
	}

	s.listenerPort = tl.Port
	return s.listener, nil
}

func (s *SshClient) ListenerPort() int {
	return s.listenerPort
}

func (s *SshClient) SetUserData(d interface{}) {
	s.userData = d
}

func (s *SshClient) UserData() interface{} {
	return s.userData
}

func (s *SshClient) NewSession() (*ssh.Session, error) {
	sess := mylog.Check2(s.client.NewSession())
	mylog.Check(prefixWithSshEndpt(s.endpt, "SshClient.NewSession", nil))
	return sess, nil
}
