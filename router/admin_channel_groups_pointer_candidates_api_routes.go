package router

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"realms/internal/store"
)

func adminListChannelGroupPointerCandidatesHandler(opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opts.Store == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "store 未初始化"})
			return
		}
		groupID, err := strconv.ParseInt(strings.TrimSpace(c.Param("group_id")), 10, 64)
		if err != nil || groupID <= 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "group_id 不合法"})
			return
		}
		if _, err := opts.Store.GetChannelGroupByID(c.Request.Context(), groupID); err != nil {
			if err == sql.ErrNoRows {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Not Found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		out, err := listChannelGroupPointerCandidates(c.Request.Context(), opts.Store, groupID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "查询失败"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
	}
}

func listChannelGroupPointerCandidates(ctx context.Context, st *store.Store, groupID int64) ([]adminChannelRefView, error) {
	if st == nil || groupID <= 0 {
		return nil, nil
	}

	visitedGroups := make(map[int64]struct{}, 32)
	seenChannels := make(map[int64]adminChannelRefView, 128)

	queue := make([]int64, 0, 32)
	queue = append(queue, groupID)
	for len(queue) > 0 {
		gid := queue[0]
		queue = queue[1:]
		if gid <= 0 {
			continue
		}
		if _, ok := visitedGroups[gid]; ok {
			continue
		}
		visitedGroups[gid] = struct{}{}

		ms, err := st.ListChannelGroupMembers(ctx, gid)
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			if m.MemberChannelID != nil && *m.MemberChannelID > 0 {
				id := *m.MemberChannelID
				if _, ok := seenChannels[id]; !ok {
					name := ""
					if m.MemberChannelName != nil {
						name = strings.TrimSpace(*m.MemberChannelName)
					}
					typ := ""
					if m.MemberChannelType != nil {
						typ = strings.TrimSpace(*m.MemberChannelType)
					}
					seenChannels[id] = adminChannelRefView{ID: id, Name: name, Type: typ}
				}
			}

			if m.MemberGroupID != nil && *m.MemberGroupID > 0 {
				if m.MemberGroupStatus != nil && *m.MemberGroupStatus != 1 {
					continue
				}
				queue = append(queue, *m.MemberGroupID)
			}
		}
	}

	out := make([]adminChannelRefView, 0, len(seenChannels))
	for _, v := range seenChannels {
		if v.ID <= 0 {
			continue
		}
		v.Name = strings.TrimSpace(v.Name)
		if v.Name == "" {
			v.Name = "channel-" + strconv.FormatInt(v.ID, 10)
		}
		v.Type = strings.TrimSpace(v.Type)
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
