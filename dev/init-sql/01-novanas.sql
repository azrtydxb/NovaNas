-- NovaNas application database
CREATE USER novanas WITH PASSWORD 'novanas';
CREATE DATABASE novanas OWNER novanas;
GRANT ALL PRIVILEGES ON DATABASE novanas TO novanas;
