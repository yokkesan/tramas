-- Provision once with the database owner; migration execution uses this non-superuser.
\getenv migrator_user MIGRATION_DB_USER
\getenv migrator_password MIGRATION_DB_PASSWORD
\getenv db_name POSTGRES_DB
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION', :'migrator_user', :'migrator_password') \gexec
GRANT CONNECT ON DATABASE :"db_name" TO :"migrator_user";
GRANT USAGE, CREATE ON SCHEMA public TO :"migrator_user";
