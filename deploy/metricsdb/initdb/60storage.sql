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

-- V1 Storage metrics table with highly temporally variable data
-- It references the more stable StorageMetadata table
CREATE TABLE IF NOT EXISTS StorageMetrics (
    Timestamp timestamptz NOT NULL,
    StorageMetadataNum integer NOT NULL REFERENCES StorageMetadata MATCH FULL ON DELETE CASCADE,
    -- metrics and analysis for this datum
    BytesRead bigint,
    BytesWritten bigint,
    NumberReads integer,
    NumberWrites integer,
    LatencyMean integer, -- Unit: Microseconds
    LatencyMax integer,  -- Unit: Microseconds
    SampleDuration smallint, -- Unit: seconds
    ViolationLatencyMean integer, -- non-zero if above compliance mean
    ViolationLatencyMax integer, -- number of times above compliance max
    ViolationWorkloadRate bigint -- quantity (IOPs or Bytes) above device limit
);
CREATE UNIQUE INDEX IF NOT EXISTS StorageMetricsI1 ON StorageMetrics (Timestamp DESC, StorageMetadataNum);
INSERT INTO SchemaVersion (TableName, Version) VALUES ('StorageMetrics', 1) ON CONFLICT DO NOTHING;

-- Convert StorageMetrics to a hypertable (note: need lowercase)
SELECT create_hypertable (
    'storagemetrics',
    'timestamp',
    chunk_time_interval => interval '7 day',
    create_default_indexes => FALSE,
    if_not_exists => TRUE);

-- V1 denormalized insertion helper
CREATE OR REPLACE FUNCTION StorageMetricsInsert1(
    pT timestamptz, pStorageID uuid,
    pBytesRead bigint, pBytesWritten bigint,
    pNumberReads integer, pNumberWrites integer,
    pLatencyMean integer, pLatencyMax integer,
    pSampleDuration smallint,
    pViolationLatencyMean integer,
    pViolationLatencyMax integer,
    pViolationWorkloadRate bigint
) RETURNS void
AS $$
DECLARE
    vmNum integer := (
        SELECT max(StorageMetadataNum)
        FROM StorageMetadata SM JOIN StorageObjects S ON SM.StorageNum = S.StorageNum
        WHERE S.StorageID = pStorageID AND SM.Timestamp <= pT);
BEGIN
    INSERT INTO StorageMetrics(
            Timestamp, StorageMetadataNum,
            BytesRead, BytesWritten,
            NumberReads, NumberWrites,
            LatencyMean, LatencyMax,
            SampleDuration,
            ViolationLatencyMean,
            ViolationLatencyMax,
            ViolationWorkloadRate
        ) VALUES (
            pT, vmNum,
            pBytesRead, pBytesWritten,
            pNumberReads, pNumberWrites,
            pLatencyMean, pLatencyMax,
            pSampleDuration,
            pViolationLatencyMean,
            pViolationLatencyMax,
            pViolationWorkloadRate
        ) ON CONFLICT DO NOTHING;
END
$$
LANGUAGE plpgsql;

-- V1 normalized metadata view
CREATE OR REPLACE VIEW StorageMetadata1 AS
SELECT MD.Timestamp, MD.StorageMetadataNum, MD.StorageNum,
    MD.StorageTypeNum, MD.DomainNum, MD.PoolNum, MD.ClusterNum,
    MD.TotalBytes, MD.AvailableBytes,
    StorageID, StorageTypeID, DomainID, PoolID, ClusterID
FROM StorageMetadata MD
JOIN StorageObjects S ON MD.StorageNum = S.StorageNum
JOIN StorageTypes ST ON MD.StorageTypeNum = ST.StorageTypeNum
JOIN DomainObjects D ON MD.DomainNum = D.DomainNum
JOIN PoolObjects P ON MD.PoolNum = P.PoolNum
JOIN ClusterObjects CL ON MD.ClusterNum = CL.ClusterNum
;
