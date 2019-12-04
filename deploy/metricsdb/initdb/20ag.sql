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

-- ApplicationGroupObjects table tracks ApplicationGroup objects
CREATE TABLE IF NOT EXISTS ApplicationGroupObjects(
    ApplicationGroupNum serial PRIMARY KEY,
    ApplicationGroupID uuid
);
CREATE UNIQUE INDEX IF NOT EXISTS ApplicationGroupObjectsI1 ON ApplicationGroupObjects(ApplicationGroupID);
INSERT INTO ApplicationGroupObjects VALUES (0, uuid_nil()) ON CONFLICT DO NOTHING; -- empty record for FK refs

-- normalization helper
CREATE OR REPLACE FUNCTION ApplicationGroupNumber (objid uuid)
    RETURNS int
AS $$
DECLARE
    num integer;
BEGIN
    SELECT
        ApplicationGroupNum INTO num
    FROM
        ApplicationGroupObjects
    WHERE
        ApplicationGroupID = objid;
    IF found THEN
        RETURN num;
    END IF;
    INSERT INTO ApplicationGroupObjects (ApplicationGroupID)
    VALUES (objid)
    ON CONFLICT DO NOTHING;
    RETURN ApplicationGroupNumber(objid);
END
$$
LANGUAGE plpgsql;
