package postgresql

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/hashicorp/errwrap"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/lib/pq"
)

const (
	schemaNameAttr  = "name"
	schemaOwnerAttr = "owner"
)

func resourcePostgreSQLSchema() *schema.Resource {
	return &schema.Resource{
		Create: resourcePostgreSQLSchemaCreate,
		Read:   resourcePostgreSQLSchemaRead,
		Update: resourcePostgreSQLSchemaUpdate,
		Delete: resourcePostgreSQLSchemaDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			schemaNameAttr: {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The name of the schema",
			},
			schemaOwnerAttr: {
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
				Description: "The ROLE name who owns the schema",
			},
		},
	}
}

func resourcePostgreSQLSchemaCreate(d *schema.ResourceData, meta interface{}) error {
	c := meta.(*Client)
	conn, err := c.Connect()
	if err != nil {
		return errwrap.Wrapf("Error connecting to PostgreSQL: {{err}}", err)
	}
	defer conn.Close()

	schemaName := d.Get(schemaNameAttr).(string)
	b := bytes.NewBufferString("CREATE SCHEMA ")
	fmt.Fprintf(b, pq.QuoteIdentifier(schemaName))

	switch v, ok := d.GetOk(schemaOwnerAttr); {
	case ok:
		fmt.Fprint(b, " AUTHORIZATION ", pq.QuoteIdentifier(v.(string)))
	}

	query := b.String()
	_, err = conn.Query(query)
	if err != nil {
		return errwrap.Wrapf(fmt.Sprintf("Error creating schema %s: {{err}}", schemaName), err)
	}

	d.SetId(schemaName)

	return resourcePostgreSQLSchemaRead(d, meta)
}

func resourcePostgreSQLSchemaDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*Client)
	conn, err := client.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	schemaName := d.Get(schemaNameAttr).(string)
	query := fmt.Sprintf("DROP SCHEMA %s", pq.QuoteIdentifier(schemaName))
	_, err = conn.Query(query)
	if err != nil {
		return errwrap.Wrapf("Error deleting schema: {{err}}", err)
	}

	d.SetId("")

	return nil
}

func resourcePostgreSQLSchemaRead(d *schema.ResourceData, meta interface{}) error {
	c := meta.(*Client)
	conn, err := c.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	schemaId := d.Id()
	var schemaName, schemaOwner string
	err = conn.QueryRow("SELECT nspname, pg_catalog.pg_get_userbyid(nspowner) FROM pg_catalog.pg_namespace WHERE nspname=$1", schemaId).Scan(&schemaName, &schemaOwner)
	switch {
	case err == sql.ErrNoRows:
		log.Printf("[WARN] PostgreSQL schema (%s) not found", schemaId)
		d.SetId("")
		return nil
	case err != nil:
		return errwrap.Wrapf("Error reading schema: {{err}}", err)
	default:
		d.Set(schemaNameAttr, schemaName)
		d.Set(schemaOwnerAttr, schemaOwner)
		d.SetId(schemaName)
		return nil
	}
}

func resourcePostgreSQLSchemaUpdate(d *schema.ResourceData, meta interface{}) error {
	c := meta.(*Client)
	conn, err := c.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := setSchemaName(conn, d); err != nil {
		return err
	}

	if err := setSchemaOwner(conn, d); err != nil {
		return err
	}

	return resourcePostgreSQLSchemaRead(d, meta)
}

func setSchemaName(conn *sql.DB, d *schema.ResourceData) error {
	if !d.HasChange(schemaNameAttr) {
		return nil
	}

	oraw, nraw := d.GetChange(schemaNameAttr)
	o := oraw.(string)
	n := nraw.(string)
	if n == "" {
		return errors.New("Error setting schema name to an empty string")
	}

	query := fmt.Sprintf("ALTER SCHEMA %s RENAME TO %s", pq.QuoteIdentifier(o), pq.QuoteIdentifier(n))
	if _, err := conn.Query(query); err != nil {
		return errwrap.Wrapf("Error updating schema NAME: {{err}}", err)
	}
	d.SetId(n)

	return nil
}

func setSchemaOwner(conn *sql.DB, d *schema.ResourceData) error {
	if !d.HasChange(schemaOwnerAttr) {
		return nil
	}

	oraw, nraw := d.GetChange(schemaOwnerAttr)
	o := oraw.(string)
	n := nraw.(string)
	if n == "" {
		return errors.New("Error setting schema owner to an empty string")
	}

	query := fmt.Sprintf("ALTER SCHEMA %s OWNER TO %s", pq.QuoteIdentifier(o), pq.QuoteIdentifier(n))
	if _, err := conn.Query(query); err != nil {
		return errwrap.Wrapf("Error updating schema OWNER: {{err}}", err)
	}

	return nil
}
