package handlers

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/phillip-england/totem/pkg/data"
	"github.com/phillip-england/vii"
)

const sessionCookieName = "totem_session"

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := normalizeRequestPath(r)
		session, user, ok := getSessionFromRequest(r)
		if ok {
			r = vii.SetContext("auth_user", user, r)
			r = vii.SetContext("auth_session", session, r)
		} else if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie != nil && cookie.Value != "" {
			clearSessionCookie(w)
		}

		if path == "/" {
			if ok {
				http.Redirect(w, r, "/admin", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(path, "/admin") && !ok {
			clearSessionCookie(w)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func normalizeRequestPath(r *http.Request) string {
	path := r.URL.Path
	if strings.HasPrefix(path, "/") {
		return path
	}
	parts := strings.SplitN(path, " ", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return path
}

func getSessionFromRequest(r *http.Request) (data.Session, data.User, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie == nil || cookie.Value == "" {
		return data.Session{}, data.User{}, false
	}
	session, err := data.GetSessionByKey(cookie.Value)
	if err != nil {
		return data.Session{}, data.User{}, false
	}
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		_ = data.DeleteSessionByKey(session.Key)
		return data.Session{}, data.User{}, false
	}
	user, err := data.GetUserByID(session.UserID)
	if err != nil {
		return data.Session{}, data.User{}, false
	}
	if isAdminUser(user) {
		token := adminSessionPayload()
		if token != "" && session.Payload != token {
			_ = data.DeleteSessionByKey(session.Key)
			return data.Session{}, data.User{}, false
		}
	} else if session.Payload != user.PasswordHash {
		_ = data.DeleteSessionByKey(session.Key)
		return data.Session{}, data.User{}, false
	}
	return session, user, true
}

func setSessionCookie(w http.ResponseWriter, key string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    key,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
	})
}

func isAdminUser(user data.User) bool {
	return user.Role == "admin"
}

func adminSessionPayload() string {
	return strings.TrimSpace(os.Getenv("ADMIN_SESSION_TOKEN"))
}

func currentUser(r *http.Request) (data.User, bool) {
	val := vii.GetContext("auth_user", r)
	user, ok := val.(data.User)
	return user, ok
}
