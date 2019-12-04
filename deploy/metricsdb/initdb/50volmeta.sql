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

-- V1 Volume metadata table with relatively stable data
CREATE TABLE IF NOT EXISTS VolumeMetadata (
    VolumeMetadataNum serial PRIMARY KEY,
    Timestamp timestamptz NOT NULL,
    VolumeNum integer REFERENCES VolumeObjects MATCH FULL ON DELETE CASCADE,
    AccountNum integer DEFAULT 0 REFERENCES AccountObjects ON DELETE SET DEFAULT, -- invariant for a given vnum
    ServicePlanNum integer DEFAULT 0 REFERENCES ServicePlanObjects ON DELETE SET DEFAULT,
    ClusterNum integer DEFAULT 0 REFERENCES ClusterObjects ON DELETE SET DEFAULT,
    ConsistencyGroupNum integer DEFAULT 0 REFERENCES ConsistencyGroupObjects ON DELETE SET DEFAULT,
    TotalBytes bigint, -- sizeBytes
    CostBytes bigint, -- spaAdditionalBytes
    BytesAllocated bigint, -- bytes claimed from SPA
    RequestedCacheSizeBytes bigint, -- desired size according to Storage Plan
    AllocatedCacheSizeBytes bigint -- actual allocated size
);
CREATE UNIQUE INDEX IF NOT EXISTS VolumeMetadataI1 ON VolumeMetadata (VolumeNum, Timestamp DESC);
INSERT INTO SchemaVersion (TableName, Version) VALUES ('VolumeMetadata', 1) ON CONFLICT DO NOTHING;


-- V1 Application group membership table tracks volume application group membership over time.
-- It is linked to the VolumeMetadata table because the data is captured at the same point in time.
-- Its schema version is tracked by the VolumeMetadata table schema version.
CREATE TABLE IF NOT EXISTS AGMember (
    VolumeMetadataNum integer NOT NULL REFERENCES VolumeMetadata MATCH FULL ON DELETE CASCADE,
    ApplicationGroupNum integer REFERENCES ApplicationGroupObjects MATCH FULL ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS AGMemberI1 ON AGMember (VolumeMetadataNum, ApplicationGroupNum);

-- V1 denormalized insertion helper
CREATE OR REPLACE FUNCTION VolumeMetadataInsert1(
    pT timestamptz,
    pVolumeID uuid,
    pAccountID uuid,
    pServicePlanID uuid,
    pClusterID uuid,
    pConsistencyGroupID uuid,
    pApplicationGroupID uuid[],
    pTotalBytes bigint,
    pCostBytes bigint,
    pBytesAllocated bigint,
    pRequestedCacheSizeBytes bigint,
    pAllocatedCacheSizeBytes bigint
) RETURNS void
AS $$
DECLARE
    vmNum integer;
    agid uuid;
BEGIN
    INSERT INTO VolumeMetadata(
        Timestamp, VolumeNum,
        AccountNum, ServicePlanNum, ClusterNum, ConsistencyGroupNum,
        TotalBytes, CostBytes, BytesAllocated, RequestedCacheSizeBytes, AllocatedCacheSizeBytes
    ) VALUES (
        pT, VolumeNumber(pVolumeID),
        AccountNumber(pAccountID), ServicePlanNumber(pServicePlanID), ClusterNumber(pClusterID),
        ConsistencyGroupNumber(pConsistencyGroupID),
        pTotalBytes, pCostBytes, pBytesAllocated, pRequestedCacheSizeBytes, pAllocatedCacheSizeBytes
    ) ON CONFLICT DO NOTHING RETURNING VolumeMetadataNum INTO vmNum;
    IF vmNum IS NOT NULL
    THEN
        FOREACH agid IN ARRAY pApplicationGroupID
        LOOP
            INSERT INTO AGMember(VolumeMetadataNum, ApplicationGroupNum)
            VALUES (vmNum, ApplicationGroupNumber(agid))
            ON CONFLICT DO NOTHING;
        END LOOP;
    END IF;
END
$$
LANGUAGE plpgsql;

-- V1 normalized metadata view (without AG)
CREATE OR REPLACE VIEW VolumeMetadata1 AS
SELECT MD.Timestamp, MD.VolumeMetadataNum, MD.VolumeNum,
    MD.AccountNum, MD.ServicePlanNum, MD.ClusterNum, MD.ConsistencyGroupNum,
    MD.TotalBytes, MD.CostBytes, MD.BytesAllocated, MD.RequestedCacheSizeBytes, MD.AllocatedCacheSizeBytes,
    VolumeID, AccountID, ServicePlanID, ClusterID, ConsistencyGroupID
FROM VolumeMetadata MD
JOIN VolumeObjects V ON MD.VolumeNum = V.VolumeNum
JOIN AccountObjects A ON MD.AccountNum = A.AccountNum
JOIN ServicePlanObjects SP ON MD.ServicePlanNum = SP.ServicePlanNum
JOIN ClusterObjects CL ON MD.ClusterNum = CL.ClusterNum
JOIN ConsistencyGroupObjects CG ON MD.ConsistencyGroupNum = CG.ConsistencyGroupNum
;
