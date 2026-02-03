package service

import (
	"embed"
	"html/template"
	"net/http"
	"sort"
)

//go:embed admin.html
var adminHTML embed.FS

// AdminPage 管理页面数据
type AdminPage struct {
	Stats       *ServerStats
	Error       string
	Success     string
	AdminToken  string
}

// 注册管理界面路由
func (s *Server) registerAdminUI(mux *http.ServeMux) {
	mux.HandleFunc("/admin", s.handleAdminUI)
	mux.HandleFunc("/admin/", s.handleAdminUI)
}

func (s *Server) handleAdminUI(w http.ResponseWriter, r *http.Request) {
	adminToken := r.URL.Query().Get("token")
	if adminToken == "" {
		// 尝试从 cookie 获取
		if cookie, err := r.Cookie("admin_token"); err == nil {
			adminToken = cookie.Value
		}
	}

	// 如果是 POST 请求处理登录
	if r.Method == "POST" {
		r.ParseForm()
		action := r.FormValue("action")

		switch action {
		case "login":
			adminToken = r.FormValue("token")
			if s.validateAdminToken(adminToken) {
				http.SetCookie(w, &http.Cookie{
					Name:     "admin_token",
					Value:    adminToken,
					Path:     "/admin",
					MaxAge:   86400 * 7, // 7 天
					HttpOnly: true,
				})
				http.Redirect(w, r, "/admin", http.StatusSeeOther)
				return
			}
			s.renderAdminPage(w, &AdminPage{Error: "无效的管理令牌"})
			return

		case "logout":
			http.SetCookie(w, &http.Cookie{
				Name:   "admin_token",
				Value:  "",
				Path:   "/admin",
				MaxAge: -1,
			})
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return

		case "create_tenant":
			if !s.validateAdminToken(adminToken) {
				http.Redirect(w, r, "/admin", http.StatusSeeOther)
				return
			}
			id := r.FormValue("id")
			name := r.FormValue("name")
			token := r.FormValue("tenant_token")
			if id == "" || name == "" || token == "" {
				s.renderAdminPageWithAuth(w, adminToken, "", "请填写完整信息")
				return
			}
			if _, err := s.CreateTenant(id, name, token); err != nil {
				s.renderAdminPageWithAuth(w, adminToken, "", err.Error())
				return
			}
			s.renderAdminPageWithAuth(w, adminToken, "租户创建成功", "")
			return

		case "delete_tenant":
			if !s.validateAdminToken(adminToken) {
				http.Redirect(w, r, "/admin", http.StatusSeeOther)
				return
			}
			id := r.FormValue("id")
			if err := s.DeleteTenant(id); err != nil {
				s.renderAdminPageWithAuth(w, adminToken, "", err.Error())
				return
			}
			s.renderAdminPageWithAuth(w, adminToken, "租户已删除", "")
			return
		}
	}

	// 验证 token
	if !s.validateAdminToken(adminToken) {
		s.renderAdminPage(w, &AdminPage{})
		return
	}

	s.renderAdminPageWithAuth(w, adminToken, "", "")
}

func (s *Server) validateAdminToken(token string) bool {
	if token == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.tenants[token]
	return exists
}

func (s *Server) renderAdminPage(w http.ResponseWriter, page *AdminPage) {
	tmpl, err := template.ParseFS(adminHTML, "admin.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, page)
}

func (s *Server) renderAdminPageWithAuth(w http.ResponseWriter, adminToken, success, errMsg string) {
	stats := s.getServerStats()
	page := &AdminPage{
		Stats:      stats,
		AdminToken: adminToken,
		Success:    success,
		Error:      errMsg,
	}
	s.renderAdminPage(w, page)
}

func (s *Server) getServerStats() *ServerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalFiles int
	var totalSize int64
	tenantStats := make([]*TenantStats, 0, len(s.tenants))

	for _, t := range s.tenants {
		var tSize int64
		for _, f := range t.Files {
			tSize += f.Size
		}
		totalFiles += len(t.Files)
		totalSize += tSize

		clients := make([]*ClientInfo, 0, len(t.Clients))
		for _, c := range t.Clients {
			clients = append(clients, c)
		}

		// 按最后活跃时间排序客户端
		sort.Slice(clients, func(i, j int) bool {
			return clients[i].LastSeen.After(clients[j].LastSeen)
		})

		tenantStats = append(tenantStats, &TenantStats{
			ID:          t.ID,
			Name:        t.Name,
			FileCount:   len(t.Files),
			TotalSize:   tSize,
			ClientCount: len(t.Clients),
			Clients:     clients,
			LastActive:  t.LastActive,
		})
	}

	// 按最后活跃时间排序租户
	sort.Slice(tenantStats, func(i, j int) bool {
		return tenantStats[i].LastActive.After(tenantStats[j].LastActive)
	})

	return &ServerStats{
		TotalTenants: len(s.tenants),
		TotalFiles:   totalFiles,
		TotalSize:    totalSize,
		Tenants:      tenantStats,
	}
}
