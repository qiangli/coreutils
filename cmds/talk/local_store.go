package talkcmd

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const endpointSchema = "dhnt-talk-local-v1"

type endpointRecord struct {
	Schema   string `json:"schema"`
	User     string `json:"user"`
	UID      string `json:"uid"`
	PeerUser string `json:"peer_user"`
	PeerUID  string `json:"peer_uid"`
	PID      int    `json:"pid"`
	Created  int64  `json:"created_unix_nano"`
	Token    string `json:"token"`
	Public   string `json:"x25519_public"`
	Socket   string `json:"socket"`
}

type wireRecord struct {
	Session string `json:"session"`
	FromUID string `json:"from_uid"`
	ToUID   string `json:"to_uid"`
	Kind    string `json:"kind"`
	Seq     uint64 `json:"seq"`
	Nonce   string `json:"nonce"`
	Data    string `json:"data"`
}

type localConversation struct {
	root, prefix, endpointPath, socketPath string
	self, peer                             account
	local, remote                          endpointRecord
	private                                *ecdh.PrivateKey
	listener                               *net.UnixConn
	remoteAddr                             *net.UnixAddr
	id                                     string
	aead                                   cipher.AEAD
	sendSeq, receivedSeq                   uint64
	joined, cleaned                        bool
}

func openLocalConversation(root string, self, peer account) (*localConversation, error) {
	if _, err := strconv.ParseUint(self.UID, 10, 32); err != nil {
		return nil, fmt.Errorf("invalid local uid %q", self.UID)
	}
	if _, err := strconv.ParseUint(peer.UID, 10, 32); err != nil {
		return nil, fmt.Errorf("invalid peer uid %q", peer.UID)
	}
	if err := validateSharedRootFn(root); err != nil {
		return nil, err
	}
	a, _ := strconv.ParseUint(self.UID, 10, 64)
	b, _ := strconv.ParseUint(peer.UID, 10, 64)
	if a > b {
		a, b = b, a
	}
	prefix := fmt.Sprintf("dhnt-talk-%d-%d-", a, b)
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	tokenBytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)
	socketName := fmt.Sprintf("%ssock-%s-%s", prefix, self.UID, token)
	socketPath := filepath.Join(root, socketName)
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: socketPath, Net: "unixgram"})
	if err != nil {
		return nil, fmt.Errorf("listen on private local socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o622); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, err
	}
	ep := endpointRecord{Schema: endpointSchema, User: self.Name, UID: self.UID,
		PeerUser: peer.Name, PeerUID: peer.UID, PID: os.Getpid(), Created: time.Now().UnixNano(),
		Token: token, Public: base64.RawStdEncoding.EncodeToString(private.PublicKey().Bytes()), Socket: socketName}
	epPath := filepath.Join(root, fmt.Sprintf("%sep-%s-%s.json", prefix, self.UID, token))
	data, _ := json.Marshal(ep)
	ef, err := os.OpenFile(epPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		_, err = ef.Write(append(data, '\n'))
		err = errors.Join(err, ef.Close())
	}
	if err == nil {
		err = os.Chmod(epPath, 0o644)
	}
	if err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		_ = os.Remove(epPath)
		return nil, err
	}
	return &localConversation{root: root, prefix: prefix, endpointPath: epPath, socketPath: socketPath,
		self: self, peer: peer, local: ep, private: private, listener: listener}, nil
}

func validateSharedRoot(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe talk session directory %s", path)
	}
	if info.Mode().Perm() != 0o777 || info.Mode()&os.ModeSticky == 0 {
		return fmt.Errorf("shared talk directory is not mode 01777: %s", path)
	}
	owner, err := fileOwnerUIDFn(path)
	if err != nil || owner != "0" {
		return fmt.Errorf("shared talk directory is not root-owned: %s", path)
	}
	return nil
}

func (c *localConversation) ID() string { return c.id }

// Join pairs endpoints by their stable rank within each account's live
// endpoint list. Thus two users who invoke talk simultaneously select the same
// pair even though each created its own invitation first.
func (c *localConversation) Join() (bool, error) {
	if c.joined {
		return true, nil
	}
	entries, err := os.ReadDir(c.root)
	if err != nil {
		return false, err
	}
	var own, peers []endpointRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), c.prefix+"ep-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		ep, valid := c.readEndpoint(filepath.Join(c.root, entry.Name()))
		if !valid {
			continue
		}
		switch {
		case ep.UID == c.self.UID && ep.PeerUID == c.peer.UID:
			own = append(own, ep)
		case ep.UID == c.peer.UID && ep.PeerUID == c.self.UID:
			peers = append(peers, ep)
		}
	}
	sort.Slice(own, func(i, j int) bool { return own[i].Token < own[j].Token })
	sort.Slice(peers, func(i, j int) bool { return peers[i].Token < peers[j].Token })
	rank := -1
	for i := range own {
		if own[i].Token == c.local.Token {
			rank = i
			break
		}
	}
	if rank < 0 {
		return false, errors.New("local endpoint lost before rendezvous")
	}
	if rank >= len(peers) {
		return false, nil
	}
	peer := peers[rank]
	peerSocket := filepath.Join(c.root, peer.Socket)
	if err := validatePeerSocket(peerSocket, peer.UID); err != nil {
		return false, err
	}
	publicBytes, err := base64.RawStdEncoding.DecodeString(peer.Public)
	if err != nil {
		return false, fmt.Errorf("invalid peer public key: %w", err)
	}
	public, err := ecdh.X25519().NewPublicKey(publicBytes)
	if err != nil {
		return false, fmt.Errorf("invalid peer public key: %w", err)
	}
	secret, err := c.private.ECDH(public)
	if err != nil {
		return false, err
	}
	tokens := []string{c.local.Token, peer.Token}
	sort.Strings(tokens)
	digest := sha256.Sum256([]byte(tokens[0] + "\x00" + tokens[1]))
	keyMaterial := append([]byte("dhnt-talk-local-v1\x00"), secret...)
	keyMaterial = append(keyMaterial, digest[:]...)
	key := sha256.Sum256(keyMaterial)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return false, err
	}
	c.aead, err = cipher.NewGCM(block)
	if err != nil {
		return false, err
	}
	c.remote = peer
	c.remoteAddr = &net.UnixAddr{Name: peerSocket, Net: "unixgram"}
	c.id = hex.EncodeToString(digest[:12])
	c.joined = true
	return true, nil
}

func (c *localConversation) readEndpoint(path string) (endpointRecord, bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 4096 {
		return endpointRecord{}, false
	}
	var ep endpointRecord
	if json.Unmarshal(data, &ep) != nil || ep.Schema != endpointSchema || ep.PID <= 0 || ep.Token == "" ||
		strings.Contains(ep.Socket, "/") || !processAliveFn(ep.PID) {
		return endpointRecord{}, false
	}
	uid, err := fileOwnerUIDFn(path)
	if err != nil || uid != ep.UID {
		return endpointRecord{}, false
	}
	acct, err := lookupAccountFn(ep.User)
	if err != nil || acct.UID != ep.UID {
		return endpointRecord{}, false
	}
	return ep, true
}

func validatePeerSocket(path, uid string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o622 {
		return fmt.Errorf("unsafe peer local socket permissions: %s", path)
	}
	owner, err := fileOwnerUIDFn(path)
	if err != nil || owner != uid {
		return fmt.Errorf("peer local socket is not owned by uid %s", uid)
	}
	return nil
}

func (c *localConversation) Send(body string) error { return c.send("data", body) }

func (c *localConversation) Close() error {
	if !c.joined {
		return nil
	}
	return c.send("close", "")
}

func (c *localConversation) send(kind, body string) error {
	if !c.joined || c.aead == nil || c.remoteAddr == nil {
		return errors.New("talk session is not joined")
	}
	if err := validatePeerSocket(c.remoteAddr.Name, c.peer.UID); err != nil {
		return err
	}
	c.sendSeq++
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	aad := []byte(c.id + "\x00" + c.self.UID + "\x00" + c.peer.UID + "\x00" + kind)
	ciphertext := c.aead.Seal(nil, nonce, []byte(body), aad)
	record := wireRecord{Session: c.id, FromUID: c.self.UID, ToUID: c.peer.UID, Kind: kind, Seq: c.sendSeq,
		Nonce: base64.RawStdEncoding.EncodeToString(nonce), Data: base64.RawStdEncoding.EncodeToString(ciphertext)}
	packet, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(packet) > 60*1024 {
		return errors.New("talk message exceeds local datagram limit")
	}
	conn, err := net.DialUnix("unixgram", nil, c.remoteAddr)
	if err != nil {
		return err
	}
	n, err := conn.Write(packet)
	err = errors.Join(err, conn.Close())
	if err == nil && n != len(packet) {
		err = io.ErrShortWrite
	}
	return err
}

func (c *localConversation) Poll() ([]string, bool, error) {
	if !c.joined || c.aead == nil {
		return nil, false, nil
	}
	if err := c.listener.SetReadDeadline(time.Now().Add(time.Millisecond)); err != nil {
		return nil, false, err
	}
	var messages []string
	closed := false
	// The socket is deliberately writable by the peer account, which also
	// means an unrelated local account can inject garbage. Authentication is
	// the boundary: discard anything that does not verify, and bound each drain
	// so a datagram flood cannot starve terminal input or cancellation.
	for packets := 0; packets < 64; packets++ {
		packet := make([]byte, 64*1024)
		n, _, err := c.listener.ReadFromUnix(packet)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return messages, closed, nil
			}
			return nil, false, err
		}
		var record wireRecord
		if err := json.Unmarshal(packet[:n], &record); err != nil {
			continue
		}
		if record.Session != c.id || record.FromUID != c.peer.UID || record.ToUID != c.self.UID ||
			(record.Kind != "data" && record.Kind != "close") || record.Seq <= c.receivedSeq {
			continue
		}
		nonce, nerr := base64.RawStdEncoding.DecodeString(record.Nonce)
		ciphertext, cerr := base64.RawStdEncoding.DecodeString(record.Data)
		if nerr != nil || cerr != nil {
			continue
		}
		aad := []byte(c.id + "\x00" + c.peer.UID + "\x00" + c.self.UID + "\x00" + record.Kind)
		plain, err := c.aead.Open(nil, nonce, ciphertext, aad)
		if err != nil {
			continue
		}
		c.receivedSeq = record.Seq
		if record.Kind == "close" {
			closed = true
		} else {
			messages = append(messages, string(plain))
		}
	}
	return messages, closed, nil
}

func (c *localConversation) Cleanup() error {
	if c.cleaned {
		return nil
	}
	c.cleaned = true
	err := c.listener.Close()
	for _, path := range []string{c.endpointPath, c.socketPath} {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			err = errors.Join(err, removeErr)
		}
	}
	return err
}
