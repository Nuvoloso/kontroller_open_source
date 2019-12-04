/* Copyright 2019 Tad Lebeck
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

\c nuvo_audit

-- V1 AuditLog table
CREATE TABLE IF NOT EXISTS AuditLog(
    RecordNum serial PRIMARY KEY,
    Timestamp timestamptz NOT NULL,
    ParentNum integer,
    Classification text,
    ObjectType text,
    ObjectID uuid,
    ObjectName text,
    RefObjectID uuid,
    TenantAccountID uuid,
    AccountID uuid,
    AccountName text,
    UserID uuid,
    AuthIdentifier text,
    Action text,
    Error boolean,
    Message text
);
CREATE INDEX IF NOT EXISTS AuditLog_OID ON AuditLog USING HASH (ObjectID);
INSERT INTO SchemaVersion(TableName, Version) VALUES ('AuditLog', 1) ON CONFLICT DO NOTHING;

-- V1 insertion helper
CREATE OR REPLACE FUNCTION AuditLogInsert1(
    pT timestamptz, parent integer, class text, oType text, oID uuid, oName text, refID uuid,
    tenantID uuid, accountID uuid, accountName text,
    userID uuid, authIdent text,
    action text, error boolean, msg text
) RETURNS int
AS $$
DECLARE
    seq int;
BEGIN
    INSERT INTO AuditLog(
            Timestamp, ParentNum, Classification, ObjectType, ObjectID, ObjectName, RefObjectID,
            TenantAccountID, AccountID, AccountName, UserID, AuthIdentifier,
            Action, Error, Message
        ) VALUES (
            pT, parent, class, oType, oID, oName, refID,
            tenantID, accountID, accountName, userID, authIdent,
            action, error, msg
        ) ON CONFLICT DO NOTHING RETURNING RecordNum INTO seq;
    RETURN seq;
END
$$
LANGUAGE plpgsql;
