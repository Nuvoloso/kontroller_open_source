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

\c nuvo_metrics

-- AccountObjects table tracks Account objects
CREATE TABLE IF NOT EXISTS AccountObjects(
    AccountNum serial PRIMARY KEY,
    AccountID uuid
);
CREATE UNIQUE INDEX IF NOT EXISTS AccountObjectsI1 ON AccountObjects(AccountID);
INSERT INTO AccountObjects VALUES (0, uuid_nil()) ON CONFLICT DO NOTHING; -- empty record for FK refs

-- normalization helper
CREATE OR REPLACE FUNCTION AccountNumber (objid uuid)
    RETURNS int
AS $$
DECLARE
    num integer;
BEGIN
    SELECT
        AccountNum INTO num
    FROM
        AccountObjects
    WHERE
        AccountID = objid;
    IF found THEN
        RETURN num;
    END IF;
    INSERT INTO AccountObjects (AccountID)
    VALUES (objid)
    ON CONFLICT DO NOTHING;
    RETURN AccountNumber(objid);
END
$$
LANGUAGE plpgsql;

