package model

import (
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/common"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"k8s.io/klog/v2"
)

var (
	DB *gorm.DB

	once sync.Once
)

type Model struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func InitDB() {
	dsn := common.DBDSN
	level := logger.Silent
	if klog.V(10).Enabled() {
		level = logger.Info
	}
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold: time.Second,
			LogLevel:      level,
			Colorful:      false,
		},
	)

	var err error
	once.Do(func() {
		cfg := &gorm.Config{
			Logger: newLogger,
		}
		if common.DBType == "sqlite" {
			DB, err = gorm.Open(sqlite.Open(dsn), cfg)
			if err != nil {
				panic("failed to connect database: " + err.Error())
			}
		}

		if common.DBType == "mysql" {
			mysqlDSN := strings.TrimPrefix(dsn, "mysql://")
			if !strings.Contains(mysqlDSN, "parseTime=") {
				separator := "?"
				if strings.Contains(mysqlDSN, "?") {
					separator = "&"
				}
				mysqlDSN = mysqlDSN + separator + "parseTime=true"
			}
			DB, err = gorm.Open(mysql.Open(mysqlDSN), cfg)
			if err != nil {
				panic("failed to connect database: " + err.Error())
			}
		}

		if common.DBType == "postgres" {
			DB, err = gorm.Open(postgres.Open(dsn), cfg)
			if err != nil {
				panic("failed to connect database: " + err.Error())
			}
		}
	})

	if DB == nil {
		panic("database connection is nil, check your DB_TYPE and DB_DSN settings")
	}

	// For SQLite we must enable foreign key enforcement explicitly.
	// SQLite has foreign key constraints defined in the schema but they are
	// not enforced unless PRAGMA foreign_keys = ON is set on the connection.
	if common.DBType == "sqlite" {
		if err := DB.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
			panic("failed to enable sqlite foreign keys: " + err.Error())
		}
	}
	// Deduplicate role_assignments before adding the unique index so
	// upgrades on clusters with pre-existing duplicates don't fail.
	deduplicateRoleAssignments()

	models := []interface{}{
		User{},
		Cluster{},
		GeneralSetting{},
		OAuthProvider{},
		Role{},
		RoleAssignment{},
		ResourceHistory{},
		ResourceTemplate{},
		PendingSession{},
	}
	for _, model := range models {
		err = DB.AutoMigrate(model)
		if err != nil {
			panic("failed to migrate database: " + err.Error())
		}
	}

	sqldb, err := DB.DB()
	if err == nil {
		sqldb.SetMaxOpenConns(common.DBMaxOpenConns)
		sqldb.SetMaxIdleConns(common.DBMaxIdleConns)
		sqldb.SetConnMaxLifetime(common.DBMaxIdleTime)
	}
}

// deduplicateRoleAssignments removes duplicate (role_id, subject_type, subject)
// rows from the role_assignments table, keeping only the row with the lowest ID.
// This must run BEFORE AutoMigrate adds the composite unique index to avoid a
// migration failure on clusters that already have duplicates.
func deduplicateRoleAssignments() {
	// Only act if the table already exists (fresh installs have no data yet).
	if !DB.Migrator().HasTable(&RoleAssignment{}) {
		return
	}

	// Find groups with more than one row for the same (role_id, subject_type, subject).
	type dupGroup struct {
		RoleID      uint   `gorm:"column:role_id"`
		SubjectType string `gorm:"column:subject_type"`
		Subject     string `gorm:"column:subject"`
		MinID       uint   `gorm:"column:min_id"`
	}
	var groups []dupGroup
	if err := DB.Raw(`
		SELECT role_id, subject_type, subject, MIN(id) AS min_id
		FROM role_assignments
		GROUP BY role_id, subject_type, subject
		HAVING COUNT(*) > 1
	`).Scan(&groups).Error; err != nil {
		klog.Warningf("deduplicateRoleAssignments: query failed (table may not exist yet): %v", err)
		return
	}
	if len(groups) == 0 {
		return
	}

	for _, g := range groups {
		res := DB.Exec(
			"DELETE FROM role_assignments WHERE role_id = ? AND subject_type = ? AND subject = ? AND id != ?",
			g.RoleID, g.SubjectType, g.Subject, g.MinID,
		)
		if res.Error != nil {
			klog.Errorf("deduplicateRoleAssignments: failed to clean duplicates for (%d, %s, %s): %v",
				g.RoleID, g.SubjectType, g.Subject, res.Error)
		} else if res.RowsAffected > 0 {
			klog.Infof("deduplicateRoleAssignments: removed %d duplicate(s) for (%d, %s, %s)",
				res.RowsAffected, g.RoleID, g.SubjectType, g.Subject)
		}
	}
}
