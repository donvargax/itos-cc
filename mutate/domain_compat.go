package mutate

import "github.com/donvargax/itos-cc/internal/mutants"

type Snapshot = mutants.Snapshot

type UnitResult = mutants.UnitResult

type CoverageEvidence = mutants.CoverageEvidence

type GoCoverageEvidence = mutants.GoCoverageEvidence

type CoverageBlock = mutants.CoverageBlock

type GoCoverageBlock = mutants.GoCoverageBlock

type LineScope = mutants.LineScope

type Mutant = mutants.Mutant

type SuiteEvidence = mutants.SuiteEvidence

type TestChange = mutants.TestChange

type previous = mutants.Previous

type previousScopes = mutants.PreviousScopes

type decidedBy = mutants.DecidedBy

type CoverageCheck = mutants.CoverageCheck

type GoCoverageCheck = mutants.GoCoverageCheck

type Freshness = mutants.Freshness

type Command = mutants.Command

type Sampled = mutants.Sampled

type SampledFile = mutants.SampledFile

type SampledMutant = mutants.SampledMutant

type FreshUnit = mutants.FreshUnit

type FreshCandidate = mutants.FreshCandidate

type FailFast = mutants.FailFast

type Stop = mutants.Stop

type FailedSelection = mutants.FailedSelection

type MutantResult = mutants.MutantResult

type StaleException = mutants.StaleException

type fileExceptions = mutants.FileExceptions

const Killed = mutants.Killed

const Survived = mutants.Survived

const Timeout = mutants.Timeout

const Uncovered = mutants.Uncovered

const ScopeOwn = mutants.ScopeOwn

const ScopeAllTests = mutants.ScopeAllTests

const ScopeListed = mutants.ScopeListed

const Fresh = mutants.Fresh

const Stale = mutants.Stale

const Missing = mutants.Missing

const StateCompleted = mutants.StateCompleted

const StateCancelled = mutants.StateCancelled

const StateUnattempted = mutants.StateUnattempted

const StateBlocked = mutants.StateBlocked

const skipped = mutants.Skipped

const annotationStart = mutants.AnnotationStart

const annotationEnd = mutants.AnnotationEnd

const ExceptionKilled = mutants.ExceptionKilled

const ExceptionChanged = mutants.ExceptionChanged

const ExceptionGone = mutants.ExceptionGone

const ExceptionMoved = mutants.ExceptionMoved

const ExceptionExcluded = mutants.ExceptionExcluded

var RunScope = mutants.RunScope

var recordedScope = mutants.RecordedScope

var testsMatch = mutants.TestsMatch

var compareTests = mutants.CompareTests

var unitID = mutants.UnitID

var PairByHash = mutants.PairByHash

var fileKeys = mutants.FileKeys

var EntriesOf = mutants.EntriesOf

var remembered = mutants.Remembered

var rememberedWithScopes = mutants.RememberedWithScopes

var usable = mutants.Usable

var markExcepted = mutants.MarkExcepted

var build = mutants.Build

var buildScoped = mutants.BuildScoped

var listedOf = mutants.ListedOf

var keepUnjudged = mutants.KeepUnjudged

var HasBroadOutcome = mutants.HasBroadOutcome

var broadScope = mutants.BroadScope

var recordSuiteEvidence = mutants.RecordSuiteEvidence

var cloneHashes = mutants.CloneHashes

var BroadChanges = mutants.BroadChanges

var BroadChangesForUnit = mutants.BroadChangesForUnit

var changedFiles = mutants.ChangedFiles

var uniqueSorted = mutants.UniqueSorted

var FreshnessOf = mutants.FreshnessOf

var unrecorded = mutants.Unrecorded

var OnSites = mutants.OnSites

var placed = mutants.Placed

var rank = mutants.Rank

var freshRank = mutants.FreshRank

var sortFreshCandidates = mutants.SortFreshCandidates

var show = mutants.Show

var annotationMarkers = mutants.AnnotationMarkers

var Annotation = mutants.Annotation

var oneLine = mutants.OneLine

var StripAnnotation = mutants.StripAnnotation

var exceptionsOf = mutants.ExceptionsOf

var locate = mutants.Locate

var applyExceptions = mutants.ApplyExceptions

var exceptedKey = mutants.ExceptedKey

var heldReasons = mutants.HeldReasons

type ListedChange = mutants.ListedChange

var hasListed = mutants.HasListed

const goCoverageEvidenceVersion = mutants.GoCoverageEvidenceVersion
const lineCoverageEvidenceVersion = mutants.LineCoverageEvidenceVersion

var strictLanguages = mutants.StrictLanguages
var StrictCoverage = mutants.StrictCoverage

const pythonSubprocesses = mutants.PythonSubprocesses

var CoverageProducer = mutants.CoverageProducer

const nodeSubprocesses = mutants.NodeSubprocesses

var TypeScriptCoverageProducer = mutants.TypeScriptCoverageProducer

const jvmSubprocesses = mutants.JvmSubprocesses

var KotlinCoverageProducer = mutants.KotlinCoverageProducer
var validCoverageProducer = mutants.ValidCoverageProducer
var evidenceKey = mutants.EvidenceKey
var evidenceVersion = mutants.EvidenceVersion
var kotlinBodyLine = mutants.KotlinBodyLine
var inInner = mutants.InInner
var goBlockInUnit = mutants.GoBlockInUnit
var goHasExecutableStatements = mutants.GoHasExecutableStatements

var GoCoverageChanges = mutants.GoCoverageChanges

type FileCheck = mutants.FileCheck
type FunctionCheck = mutants.FunctionCheck

var NeedsMutationCoverage = mutants.NeedsMutationCoverage

type lineRange = mutants.LineRange

var hunkRanges = mutants.HunkRanges
var overlapsRanges = mutants.OverlapsRanges
