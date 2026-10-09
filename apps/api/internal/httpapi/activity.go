package httpapi

import (
	"net/http"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/manage"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/provision"
)

// activityRecent is how long a finished job stays visible as "complete" or "failed".
const activityRecent = 10 * time.Minute

// activityJobTypes are the jobs the website list reports.
var activityJobTypes = []string{
	provision.JobType, manage.JobBackup, manage.JobRestore, manage.JobMigrate, manage.JobDelete,
}

// siteActivityView is what one website is doing now, or just finished doing.
type siteActivityView struct {
	SiteID string `json:"site_id"`
	JobID  string `json:"job_id"`
	Type   string `json:"type"`
	Status string `json:"status"`
	// Percent is 0 to 100: finished steps count fully, the running step counts half.
	Percent int `json:"percent"`
	// Step is the name of the step that is running, or the next one when none is. Empty when finished.
	Step string `json:"step"`
}

// handleSiteActivity lists what each of the caller's websites is doing: a build, a backup, a
// restore, a migration or a deletion that is running or finished in the last few minutes.
func (d Deps) handleSiteActivity(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	owner := user.ID
	if isAdmin(user) {
		owner = ""
	}
	list, err := d.Sites.ListSiteActivity(r.Context(), owner, activityJobTypes, activityRecent)
	if err != nil {
		d.internal(w, r, "sites.activity", err)
		return
	}
	out := make([]siteActivityView, 0, len(list))
	for _, a := range list {
		statuses := make([]string, 0, len(a.Steps))
		step := ""
		for _, s := range a.Steps {
			statuses = append(statuses, s.Status)
			if step == "" && s.Status == "running" {
				step = s.Name
			}
		}
		if step == "" && (a.Status == "queued" || a.Status == "running") {
			for _, s := range a.Steps {
				if s.Status == "pending" {
					step = s.Name
					break
				}
			}
		}
		percent := manage.ProgressPercent(statuses)
		if a.Status == "succeeded" {
			percent = 100
		}
		out = append(out, siteActivityView{
			SiteID: a.SiteID, JobID: a.JobID, Type: a.Type, Status: a.Status, Percent: percent, Step: step,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
