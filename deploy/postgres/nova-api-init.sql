-- Run as the postgres superuser on the NAS host:
--   sudo -u postgres psql -f /usr/share/nova-api/nova-api-init.sql

CREATE ROLE novanas WITH LOGIN PASSWORD 'novanas';
CREATE DATABASE novanas OWNER novanas;
\c novanas
GRANT ALL ON SCHEMA public TO novanas;
