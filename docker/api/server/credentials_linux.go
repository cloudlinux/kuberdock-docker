// +build linux

package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os/user"
	"path"
	"reflect"
	"strconv"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/container"
	"github.com/docker/docker/pkg/audit"
	"github.com/docker/docker/pkg/version"
	"github.com/docker/engine-api/types/versions/v1p20"
)

//Gets the file descriptor
func getFdFromWriter(w http.ResponseWriter) int {
	//We must use introspection to pull the
	//connection from the ResponseWriter object
	//This is because the connection object is not exported by the writer.
	writerVal := reflect.Indirect(reflect.ValueOf(w))
	if writerVal.Kind() != reflect.Struct {
		logrus.Warn("ResponseWriter is not a struct")
		return -1
	}
	//Get the underlying http connection
	httpconn := writerVal.FieldByName("conn")
	if !httpconn.IsValid() {
		logrus.Warn("ResponseWriter does not contain a field named conn")
		return -1
	}
	httpconnVal := reflect.Indirect(httpconn)
	if httpconnVal.Kind() != reflect.Struct {
		logrus.Warn("conn is not an interface to a struct")
		return -1
	}
	//Get the underlying tcp connection
	rwcPtr := httpconnVal.FieldByName("rwc").Elem()
	rwc := reflect.Indirect(rwcPtr)
	if rwc.Kind() != reflect.Struct {
		logrus.Warn("conn is not an interface to a struct")
		return -1
	}
	tcpconn := reflect.Indirect(rwc.FieldByName("conn"))
	//Grab the underyling netfd
	if tcpconn.Kind() != reflect.Struct {
		logrus.Warn("tcpconn is not a struct")
		return -1
	}
	netfd := reflect.Indirect(tcpconn.FieldByName("fd"))
	//Grab sysfd
	if netfd.Kind() != reflect.Struct {
		logrus.Warn("fd is not a struct")
		return -1
	}
	sysfd := netfd.FieldByName("sysfd")
	//Finally, we have the fd
	return int(sysfd.Int())
}

//Gets the ucred given an http response writer
func getUcred(fd int) (*syscall.Ucred, error) {
	return syscall.GetsockoptUcred(fd, syscall.SOL_SOCKET, syscall.SO_PEERCRED)
}

//Gets the client's loginuid
func getLoginUID(ucred *syscall.Ucred, fd int) (int64, error) {
	if _, err := syscall.Getpeername(fd); err != nil {
		logrus.Errorf("Socket appears to have closed: %v", err)
		return -1, err
	}
	loginuid, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/loginuid", ucred.Pid))
	if err != nil {
		logrus.Errorf("Error reading loginuid: %v", err)
		return -1, err
	}
	loginuidInt, err := strconv.ParseInt(string(loginuid), 10, 0)
	if err != nil {
		logrus.Errorf("Failed to convert loginuid to int: %v", err)
		return -1, err
	}
	return loginuidInt, nil
}

//Given a loginUID, retrieves the current username
func getpwuid(loginUID uint32) (string, error) {
	uid := strconv.FormatUint(uint64(loginUID), 10)
	pwd, err := user.LookupId(uid)
	if err != nil {
		logrus.Errorf("Failed to get pwuid struct: %v", err)
		return "", err
	}
	if pwd == nil {
		return "", user.UnknownUserIdError(loginUID)
	}
	name := pwd.Username
	return name, nil
}

//Retrieves the container and "action" (start, stop, kill, etc) from the http request
func (s *Server) parseRequest(r *http.Request) (string, *container.Container) {
	var (
		containerID string
		action      string
	)
	requrl := r.RequestURI
	parsedurl, err := url.Parse(requrl)
	if err != nil {
		return "?", nil
	}

	switch r.Method {
	//Delete requests do not explicitly state the action, so we check the HTTP method instead
	case "DELETE":
		action = "remove"
		containerID = path.Base(parsedurl.Path)
	default:
		action = path.Base(parsedurl.Path)
		containerID = path.Base(path.Dir(parsedurl.Path))
	}

	if s.daemon != nil {
		c, err := s.daemon.GetContainer(containerID)
		if err == nil {
			return action, c
		}
	}
	return action, nil
}

//Traverses the config struct and grabs non-standard values for logging
func parseConfig(config interface{}) string {
	configReflect := reflect.Indirect(reflect.ValueOf(config))
	var result bytes.Buffer
	for index := 0; index < configReflect.NumField(); index++ {
		val := reflect.Indirect(configReflect.Field(index))
		//Get the zero value of the struct's field
		if val.IsValid() {
			zeroVal := reflect.Zero(val.Type()).Interface()
			//If the configuration value is not a zero value, then we store it
			//We use deep equal here because some types cannot be compared with the standard equality operators
			if val.Kind() == reflect.Bool || !reflect.DeepEqual(zeroVal, val.Interface()) {
				fieldName := configReflect.Type().Field(index).Name
				if result.Len() > 0 {
					result.WriteString(", ")
				}
				fmt.Fprintf(&result, "%s=%+v", fieldName, val.Interface())
			}
		}
	}
	return result.String()
}

//Constructs a partial log message containing the container's configuration settings
func generateContainerConfigMsg(c *container.Container, j *v1p20.ContainerJSON) string {
	if c != nil && j != nil {
		configStripped := parseConfig(*c.Config)
		hostConfigStripped := parseConfig(*j.HostConfig)
		return fmt.Sprintf("Config={%v}, HostConfig={%v}", configStripped, hostConfigStripped)
	}
	return ""
}

//LogAction logs a docker API function and records the user that initiated the request using the authentication results
func (s *Server) LogAction(w http.ResponseWriter, r *http.Request) error {
	var (
		message  string
		username string
		loginuid int64 = -1
	)

	action, c := s.parseRequest(r)

	switch action {
	case "start":
		if s.daemon != nil && c != nil {
			version := version.Version("1.20")
			inspect, err := s.daemon.ContainerInspect(c.ID, false, version)
			if err == nil {
				message = ", " + generateContainerConfigMsg(c, inspect.(*v1p20.ContainerJSON))
			}
		}
		fallthrough
	default:
		//Get user credentials
		fd := getFdFromWriter(w)
		server, err := syscall.Getsockname(fd)
		if err != nil {
			logrus.Errorf("Unable to read peer creds and server socket address: %v", err)
			message = "LoginUID unknown, PID unknown" + message
			break
		}
		if _, isUnix := server.(*syscall.SockaddrUnix); !isUnix {
			logrus.Debug("Unable to read peer creds: server socket is not a Unix socket")
			message = "LoginUID unknown, PID unknown" + message
			break
		}
		ucred, err := getUcred(fd)
		if err != nil {
			logrus.Errorf("Unable to read peer creds: %v", err)
			message = "LoginUID unknown, PID unknown" + message
			break
		}
		message = fmt.Sprintf("PID=%v", ucred.Pid) + message

		//Get user loginuid
		loginuid, err = getLoginUID(ucred, fd)
		if err != nil {
			break
		}
		message = fmt.Sprintf("LoginUID=%v, %s", loginuid, message)
		if loginuid < 0 || loginuid >= 0xffffffff { // -1 means no login user
			//No login UID is set, so no point in looking up a name
			break
		}

		//Get username
		username, err = getpwuid(uint32(loginuid))
		if err != nil {
			break
		}

		message = fmt.Sprintf("Username=%v, %s", username, message)
	}

	//Log the container ID being affected if it exists
	if c != nil {
		message = fmt.Sprintf("ID=%v, %s", c.ID, message)
	}
	message = fmt.Sprintf("{Action=%v, %s}", action, message)
	// Log info messages at Debug Level
	// Log messages that change state at Info level
	switch action {
	case "info":
	case "images":
	case "version":
	case "json":
	case "search":
	case "stats":
	case "events":
	case "history":
		logrus.Debug(message)
		fallthrough
	default:
		logrus.Info(message)
		logAuditlog(c, action, username, loginuid, true)
	}
	return nil
}

//Logs an API event to the audit log
func logAuditlog(c *container.Container, action string, username string, loginuid int64, success bool) {
	virt := audit.AuditVirtControl
	vm := "?"
	vmPid := "?"
	exe := "?"
	hostname := "?"
	user := "?"
	auid := "?"

	if c != nil {
		vm = c.Config.Image
		vmPid = fmt.Sprint(c.State.Pid)
		exe = c.Path
		hostname = c.Config.Hostname
	}

	if username != "" {
		user = username
	}

	if loginuid != -1 {
		auid = fmt.Sprint(loginuid)
	}

	vars := map[string]string{
		"op":       action,
		"reason":   "api",
		"vm":       vm,
		"vm-pid":   vmPid,
		"user":     user,
		"auid":     auid,
		"exe":      exe,
		"hostname": hostname,
	}

	//Encoding is a function of libaudit that ensures
	//that the audit values contain only approved characters.
	for key, value := range vars {
		if audit.ValueNeedsEncoding(value) {
			vars[key] = audit.EncodeNVString(key, value)
		}
	}
	message := audit.FormatVars(vars)
	audit.LogUserEvent(virt, message, success)
}
