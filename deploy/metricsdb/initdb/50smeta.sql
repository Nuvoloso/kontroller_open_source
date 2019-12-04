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

-- V1 Storage metadata table with relatively stable data
CREATE TABLE IF NOT EXISTS StorageMetadata (
    StorageMetadataNum serial PRIMARY KEY,
    Timestamp timestamptz NOT NULL,
    StorageNum integer REFERENCES StorageObjects MATCH FULL ON DELETE CASCADE,
    StorageTypeNum integer REFERENCES StorageTypes MATCH FULL ON DELETE CASCADE,
    DomainNum integer REFERENCES DomainObjects MATCH FULL ON DELETE CASCADE,
    PoolNum integer REFERENCES PoolObjects MATCH FULL ON DELETE CASCADE,
    ClusterNum integer DEFAULT 0 REFERENCES ClusterObjects ON DELETE SET DEFAULT,
    TotalBytes bigint,
    AvailableBytes bigint
);
CREATE UNIQUE INDEX IF NOT EXISTS StorageMetadataI1 ON StorageMetadata (StorageNum, Timestamp DESC);
INSERT INTO SchemaVersion (TableName, Version) VALUES ('StorageMetadata', 1) ON CONFLICT DO NOTHING;

-- V1 denormalized insertion helper
CREATE OR REPLACE FUNCTION StorageMetadataInsert1(
    pT timestamptz, pStorageID uuid,
    pStorageTypeID varchar,
    pDomID uuid,
    pPoolID uuid,
    pClusterID uuid,
    pTotalBytes bigint,
    pAvailableBytes bigint
) RETURNS void
AS $$
BEGIN
    INSERT INTO StorageMetadata(
        Timestamp, StorageNum,
        StorageTypeNum, DomainNum, PoolNum, ClusterNum,
        TotalBytes, AvailableBytes
    ) VALUES (
        pT, StorageNumber(pStorageID),
        StorageTypeNumber(pStorageTypeID),
        DomainNumber(pDomID),
        PoolNumber(pPoolID),
        ClusterNumber(pClusterID),
        pTotalBytes, pAvailableBytes
    ) ON CONFLICT DO NOTHING;
END
$$
LANGUAGE plpgsql;

