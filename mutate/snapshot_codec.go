package mutate

import "github.com/donvargax/itos-cc/internal/metricsdir"

// These compatibility entry points retain the existing concrete codec owner.
var SnapshotJSON = metricsdir.SnapshotJSON
var UnitJSON = metricsdir.UnitJSON
var MutantJSON = metricsdir.MutantJSON
var ReadSnapshotJSON = metricsdir.ReadSnapshotJSON
var readUnitJSON = metricsdir.ReadUnitJSON
var readMutantJSON = metricsdir.ReadMutantJSON
