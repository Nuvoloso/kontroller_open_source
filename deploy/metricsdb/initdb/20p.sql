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

-- PoolObjects table (Pool)
CREATE TABLE IF NOT EXISTS PoolObjects(
    PoolNum serial PRIMARY KEY,
    PoolID uuid
);
CREATE UNIQUE INDEX IF NOT EXISTS PoolObjectsI1 ON PoolObjects(PoolID);
INSERT INTO PoolObjects VALUES (0, uuid_nil()) ON CONFLICT DO NOTHING; -- empty record for FK refs

-- normalization helper
CREATE OR REPLACE FUNCTION PoolNumber (objid uuid)
    RETURNS int
AS $$
DECLARE
    num integer;
BEGIN
    SELECT
        PoolNum INTO num
    FROM
        PoolObjects
    WHERE
        PoolID = objid;
    IF found THEN
        RETURN num;
    END IF;
    INSERT INTO PoolObjects (PoolID)
    VALUES (objid)
    ON CONFLICT DO NOTHING;
    RETURN PoolNumber(objid);
END
$$
LANGUAGE plpgsql;
