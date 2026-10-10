package server

import (
	"context"
	"net/http"

	"github.com/shimmerglass/musikat/database"
	"github.com/shimmerglass/musikat/server/auth"
	"github.com/shimmerglass/musikat/server/component"
)

func (s *Server) accountPage(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	return component.Account().Render(ctx, rw)
}

func (s *Server) accountUpdate(ctx context.Context, rw http.ResponseWriter, r *http.Request, user database.User) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}

	switch r.Form.Get("type") {
	case "info":
		user.Name = r.Form.Get("name")
	case "subsonic":
		user.SubsonicUser = r.Form.Get("subsonic_user")
		user.SubsonicPass = r.Form.Get("subsonic_pass")
	case "notifications":
		user.XMPPJID = r.Form.Get("xmpp_jid")
	case "password":
		if r.Form.Get("new_pass") != r.Form.Get("confirm_pass") {
			return badRequestf("passwords do not match")
		}

		ok, err := auth.ComparePasswordAndHash(r.Form.Get("current_pass"), user.Password)
		if err != nil {
			return err
		}
		if !ok {
			return badRequestf("invalid current password")
		}

		hash, err := auth.HashPass(r.Form.Get("new_pass"))
		if err != nil {
			return err
		}

		user.Password = hash
	default:
		return badRequestf("unknown update type")
	}

	err = s.db.UpdateUser(ctx, user)
	if err != nil {
		return err
	}

	http.Redirect(rw, r, "/account", http.StatusSeeOther)
	return nil
}
