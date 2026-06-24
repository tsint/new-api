package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetAllQuotaDates(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	granularity := c.DefaultQuery("granularity", "hour")

	if granularity != "quarter" && granularity != "hour" && granularity != "day" && granularity != "week" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的 granularity 参数",
		})
		return
	}

	dates, err := model.GetAllQuotaDates(startTimestamp, endTimestamp, username, granularity)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetQuotaDatesByUser(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	metric := c.DefaultQuery("metric", "token")
	granularity := c.DefaultQuery("granularity", "quarter")

	if metric != "quota" && metric != "token" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的 metric 参数",
		})
		return
	}
	if granularity != "quarter" && granularity != "hour" && granularity != "day" && granularity != "week" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的 granularity 参数",
		})
		return
	}

	dates, err := model.GetQuotaDataGroupByUser(startTimestamp, endTimestamp, granularity)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
		"metric":  metric,
	})
	return
}

func GetUserQuotaDates(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	granularity := c.DefaultQuery("granularity", "hour")

	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	if granularity != "quarter" && granularity != "hour" && granularity != "day" && granularity != "week" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的 granularity 参数",
		})
		return
	}

	dates, err := model.GetQuotaDataByUserId(userId, startTimestamp, endTimestamp, granularity)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}
