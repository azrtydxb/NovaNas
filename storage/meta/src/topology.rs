//! Single-host topology used to feed CRUSH.
//!
//! Failure domain in this design is **the disk** (a single host has only one
//! node). The topology is a flat list of `(disk_uuid, weight)` per pool.

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct DiskCandidate {
    pub disk_uuid: String,
    pub weight: u64,
}

/// A flat list of disks eligible for placement within a single pool.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct PoolTopology {
    pub pool_uuid: String,
    pub disks: Vec<DiskCandidate>,
}

impl PoolTopology {
    pub fn new(pool_uuid: impl Into<String>) -> Self {
        Self {
            pool_uuid: pool_uuid.into(),
            disks: Vec::new(),
        }
    }

    pub fn push(&mut self, disk_uuid: impl Into<String>, weight: u64) {
        self.disks.push(DiskCandidate {
            disk_uuid: disk_uuid.into(),
            weight: weight.max(1),
        });
    }

    pub fn len(&self) -> usize {
        self.disks.len()
    }

    pub fn is_empty(&self) -> bool {
        self.disks.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn topology_push_clamps_weight_to_one() {
        let mut t = PoolTopology::new("p");
        t.push("d", 0);
        assert_eq!(t.disks[0].weight, 1);
    }

    #[test]
    fn topology_len() {
        let mut t = PoolTopology::new("p");
        assert!(t.is_empty());
        t.push("a", 1);
        t.push("b", 1);
        assert_eq!(t.len(), 2);
    }
}
