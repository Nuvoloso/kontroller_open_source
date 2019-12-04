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

-- ServicePlanObjects table (Service Plan)
CREATE TABLE IF NOT EXISTS ServicePlanObjects(
    ServicePlanNum serial PRIMARY KEY,
    ServicePlanID uuid
);
CREATE UNIQUE INDEX IF NOT EXISTS ServicePlanObjectsI1 ON ServicePlanObjects(ServicePlanID);
INSERT INTO ServicePlanObjects VALUES (0, uuid_nil()) ON CONFLICT DO NOTHING; -- empty record for FK refs

-- normalization helper
CREATE OR REPLACE FUNCTION ServicePlanNumber (objid uuid)
    RETURNS int
AS $$
DECLARE
    num integer;
BEGIN
    SELECT
        ServicePlanNum INTO num
    FROM
        ServicePlanObjects
    WHERE
        ServicePlanID = objid;
    IF found THEN
        RETURN num;
    END IF;
    INSERT INTO ServicePlanObjects (ServicePlanID)
    VALUES (objid)
    ON CONFLICT DO NOTHING;
    RETURN ServicePlanNumber(objid);
END
$$
LANGUAGE plpgsql;

-- Service plan data contains (relatively static) data from service plans that may be
-- useful to display in GUI queries.
-- This should be updated on startup of centrald.
CREATE TABLE IF NOT EXISTS ServicePlanData(
    ServicePlanNum integer PRIMARY KEY REFERENCES ServicePlanObjects ON DELETE CASCADE,
    ResponseTimeAverage integer, -- Unit: Microseconds
    ResponseTimeMax integer,     -- Unit: Microseconds
    IOPSPerGiB integer,
    BytesPerSecPerGiB bigint,
    MinReadPercent integer,
    MaxReadPercent integer,
    MinAvgSizeBytes integer,
    MaxAvgSizeBytes integer
);
INSERT INTO SchemaVersion (TableName, Version) VALUES ('ServicePlanData', 1) ON CONFLICT DO NOTHING;

-- V1 denormalized upsert helper
CREATE OR REPLACE FUNCTION ServicePlanDataUpsert1 (
        pServicePlanID uuid,
        pResponseTimeAverage integer,
        pResponseTimeMax integer,
        pIOPSPerGiB integer,
        pBytesPerSecPerGiB bigint,
        pMinReadPercent integer,
        pMaxReadPercent integer,
        pMinAvgSizeBytes integer,
        pMaxAvgSizeBytes integer
    ) RETURNS void
AS $$
BEGIN
    INSERT INTO ServicePlanData (ServicePlanNum, ResponseTimeAverage, ResponseTimeMax, IOPSPerGiB, BytesPerSecPerGiB, MinReadPercent, MaxReadPercent, MinAvgSizeBytes, MaxAvgSizeBytes)
    VALUES (ServicePlanNumber(pServicePlanID), pResponseTimeAverage, pResponseTimeMax, pIOPSPerGiB, pBytesPerSecPerGiB, pMinReadPercent, pMaxReadPercent, pMinAvgSizeBytes, pMaxAvgSizeBytes)
    ON CONFLICT (ServicePlanNum) DO UPDATE SET
        (ResponseTimeAverage, ResponseTimeMax, IOPSPerGiB, BytesPerSecPerGiB, MinReadPercent, MaxReadPercent, MinAvgSizeBytes, MaxAvgSizeBytes)
            = (pResponseTimeAverage, pResponseTimeMax, pIOPSPerGiB, pBytesPerSecPerGiB, pMinReadPercent, pMaxReadPercent, pMinAvgSizeBytes, pMaxAvgSizeBytes);
END
$$
LANGUAGE plpgsql;

-- V1 denormalized view
CREATE OR REPLACE VIEW ServicePlanData1 AS
SELECT
    D.ServicePlanNum,
    ServicePlanID,
    ResponseTimeAverage,
    ResponseTimeMax,
    IOPSPerGiB,
    BytesPerSecPerGiB,
    MinReadPercent,
    MaxReadPercent,
    MinAvgSizeBytes,
    MaxAvgSizeBytes
FROM
    ServicePlanData D
    JOIN ServicePlanObjects ServicePlan ON D.ServicePlanNum = ServicePlan.ServicePlanNum
;
