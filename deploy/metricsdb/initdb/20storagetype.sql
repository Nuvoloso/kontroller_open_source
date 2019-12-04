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

-- StorageType table
CREATE TABLE IF NOT EXISTS StorageTypes(
    StorageTypeNum serial PRIMARY KEY,
    StorageTypeID varchar NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS StorageTypesI1 ON StorageTypes(StorageTypeID);
INSERT INTO StorageTypes VALUES (0, '') ON CONFLICT DO NOTHING; -- empty record for FK refs

-- normalization helper
CREATE OR REPLACE FUNCTION StorageTypeNumber (storageTypeName varchar)
    RETURNS int
AS $$
DECLARE
    num integer;
BEGIN
    SELECT
        StorageTypeNum INTO num
    FROM
        StorageTypes
    WHERE
        StorageTypeID = storageTypeName;
    IF found THEN
        RETURN num;
    END IF;
    INSERT INTO StorageTypes (StorageTypeID)
    VALUES (storageTypeName)
    ON CONFLICT DO NOTHING;
    RETURN StorageTypeNumber(storageTypeName);
END
$$
LANGUAGE plpgsql;

-- Storage type data contains the static data from storage types that may be
-- useful to display in GUI queries
CREATE TABLE IF NOT EXISTS StorageTypeData(
    StorageTypeNum integer PRIMARY KEY REFERENCES StorageTypes ON DELETE CASCADE,
    ResponseTimeAverage integer, -- Unit: Microseconds
    ResponseTimeMaximum integer, -- Unit: Microseconds
    IOPSPerGiB integer,
    BytesPerSecPerGiB bigint
);
INSERT INTO SchemaVersion (TableName, Version) VALUES ('StorageTypeData', 1) ON CONFLICT DO NOTHING;

-- V1 denormalized upsert helper
CREATE OR REPLACE FUNCTION StorageTypeDataUpsert1 (
        pStorageTypeID varchar,
        pResponseTimeAverage integer,
        pResponseTimeMaximum integer,
        pIOPSPerGiB integer,
        pBytesPerSecPerGiB bigint
    ) RETURNS void
AS $$
BEGIN
    INSERT INTO StorageTypeData (StorageTypeNum, ResponseTimeAverage, ResponseTimeMaximum, IOPSPerGiB, BytesPerSecPerGiB)
    VALUES (StorageTypeNumber(pStorageTypeID), pResponseTimeAverage, pResponseTimeMaximum, pIOPSPerGiB, pBytesPerSecPerGiB)
    ON CONFLICT (StorageTypeNum) DO UPDATE SET
        (ResponseTimeAverage, ResponseTimeMaximum, IOPSPerGiB, BytesPerSecPerGiB)
            = (pResponseTimeAverage, pResponseTimeMaximum, pIOPSPerGiB, pBytesPerSecPerGiB);
END
$$
LANGUAGE plpgsql;

-- V1 denormalized view
CREATE OR REPLACE VIEW StorageTypeData1 AS
SELECT
    D.StorageTypeNum,
    StorageTypeID,
    ResponseTimeAverage,
    ResponseTimeMaximum,
    IOPSPerGiB,
    BytesPerSecPerGiB
FROM
    StorageTypeData D
    JOIN StorageTypes ST ON D.StorageTypeNum = ST.StorageTypeNum
;
