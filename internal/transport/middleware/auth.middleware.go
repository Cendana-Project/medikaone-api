package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/repository/user"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

// key builder (local, simple)
func accessBlacklistKey(jti string) string {
	return config.AuthRedisKeyPrefix() + "access:blacklist:" + jti
}

func accessFamilyRevokedKey(userID, familyID string) string {
	return config.AuthRedisKeyPrefix() + "access:family-revoked:" + userID + ":" + familyID
}

// AuthRequired validates an HS256 access token, its session version, account,
// and blacklist state.
func AuthRequired(rdb *redis.Client, users *user.Repository) gin.HandlerFunc {
	secret := []byte(config.Env.JWT.Secret)

	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(strings.ToLower(h), "bearer ") {
			res := constant.ErrUnauthorized.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}
		raw := strings.TrimSpace(h[7:])
		if len(raw) == 0 || len(raw) > 4096 {
			res := constant.ErrInvalidToken.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}

		tok, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
			// only HS256
			if t.Method != jwt.SigningMethodHS256 {
				return nil, jwt.ErrSignatureInvalid
			}
			return secret, nil
		},
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithExpirationRequired(),
		)
		if err != nil || !tok.Valid {
			res := constant.ErrInvalidToken.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}

		claims, ok := tok.Claims.(jwt.MapClaims)
		if !ok || claims["typ"] != "access" {
			res := constant.ErrInvalidToken.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}

		sub, _ := claims["sub"].(string)
		jti, _ := claims["jti"].(string)
		familyID, _ := claims["fid"].(string)
		if sub == "" || jti == "" || familyID == "" {
			res := constant.ErrInvalidToken.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}

		if rdb == nil {
			res := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}

		blacklisted, redisErr := rdb.Exists(
			c.Request.Context(),
			accessBlacklistKey(jti),
			accessFamilyRevokedKey(sub, familyID),
		).Result()
		if redisErr != nil {
			res := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}
		if blacklisted > 0 {
			res := constant.ErrUnauthorized.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}

		tokenVersion, ok := claims["sv"].(string)
		if !ok || tokenVersion == "" {
			res := constant.ErrInvalidToken.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}
		currentVersion, versionErr := rdb.Get(c.Request.Context(), sessionVersionKey(sub)).Result()
		if versionErr == redis.Nil {
			res := constant.ErrInvalidToken.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}
		if versionErr != nil {
			res := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}
		if tokenVersion != currentVersion {
			res := constant.ErrInvalidToken.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}

		if users == nil {
			res := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}
		account, userErr := users.GetByID(c.Request.Context(), sub)
		if errors.Is(userErr, gorm.ErrRecordNotFound) {
			res := constant.ErrUnauthorized.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}
		if userErr != nil {
			res := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}
		if account == nil || !strings.EqualFold(account.Status, "active") {
			res := constant.ErrUnauthorized.ToResponse()
			util.HandleResponse(c, &res, nil)
			c.Abort()
			return
		}

		c.Set(string(constant.UserID), sub)
		c.Set(string(constant.TokenID), jti)
		requestContext := context.WithValue(c.Request.Context(), constant.UserID, sub)
		requestContext = context.WithValue(requestContext, constant.TokenID, jti)
		c.Request = c.Request.WithContext(requestContext)
		c.Next()
	}
}

func sessionVersionKey(userID string) string {
	return config.AuthRedisKeyPrefix() + "user:session-version:" + userID
}
