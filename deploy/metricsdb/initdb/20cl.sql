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

-- ClusterObjects table tracks Cluster objects
CREATE TABLE IF NOT EXISTS ClusterObjects(
    ClusterNum serial PRIMARY KEY,
    ClusterID uuid
);
CREATE UNIQUE INDEX IF NOT EXISTS ClusterObjectsI1 ON ClusterObjects(ClusterID);
INSERT INTO ClusterObjects VALUES (0, uuid_nil()) ON CONFLICT DO NOTHING;  -- empty record for FK refs

-- normalization helper
CREATE OR REPLACE FUNCTION ClusterNumber (objid uuid)
    RETURNS int
AS $$
DECLARE
    num integer;
BEGIN
    SELECT
        ClusterNum INTO num
    FROM
        ClusterObjects
    WHERE
        ClusterID = objid;
    IF found THEN
        RETURN num;
    END IF;
    INSERT INTO ClusterObjects (ClusterID)
    VALUES (objid)
    ON CONFLICT DO NOTHING;
    RETURN ClusterNumber(objid);
END
$$
LANGUAGE plpgsql;
