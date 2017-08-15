// Copyright 2017 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// ATA response parsing

package smart

// Table 10 of X3T13/2008D (ATA-3) Revision 7b, January 27, 1997
// Table 28 of T13/1410D (ATA/ATAPI-6) Revision 3b, February 26, 2002
// Table 31 of T13/1699-D (ATA8-ACS) Revision 6a, September 6, 2008
// Table 46 of T13/BSR INCITS 529 (ACS-4) Revision 08, April 28, 2015
var ataMinorVersions = map[uint16]string{
	0x0001: "ATA-1 X3T9.2/781D prior to revision 4",
	0x0002: "ATA-1 published, ANSI X3.221-1994",
	0x0003: "ATA-1 X3T9.2/781D revision 4",
	0x0004: "ATA-2 published, ANSI X3.279-1996",
	0x0005: "ATA-2 X3T10/948D prior to revision 2k",
	0x0006: "ATA-3 X3T10/2008D revision 1",
	0x0007: "ATA-2 X3T10/948D revision 2k",
	0x0008: "ATA-3 X3T10/2008D revision 0",
	0x0009: "ATA-2 X3T10/948D revision 3",
	0x000a: "ATA-3 published, ANSI X3.298-1997",
	0x000b: "ATA-3 X3T10/2008D revision 6",
	0x000c: "ATA-3 X3T13/2008D revision 7 and 7a",
	0x000d: "ATA/ATAPI-4 X3T13/1153D revision 6",
	0x000e: "ATA/ATAPI-4 T13/1153D revision 13",
	0x000f: "ATA/ATAPI-4 X3T13/1153D revision 7",
	0x0010: "ATA/ATAPI-4 T13/1153D revision 18",
	0x0011: "ATA/ATAPI-4 T13/1153D revision 15",
	0x0012: "ATA/ATAPI-4 published, ANSI NCITS 317-1998",
	0x0013: "ATA/ATAPI-5 T13/1321D revision 3",
	0x0014: "ATA/ATAPI-4 T13/1153D revision 14",
	0x0015: "ATA/ATAPI-5 T13/1321D revision 1",
	0x0016: "ATA/ATAPI-5 published, ANSI NCITS 340-2000",
	0x0017: "ATA/ATAPI-4 T13/1153D revision 17",
	0x0018: "ATA/ATAPI-6 T13/1410D revision 0",
	0x0019: "ATA/ATAPI-6 T13/1410D revision 3a",
	0x001a: "ATA/ATAPI-7 T13/1532D revision 1",
	0x001b: "ATA/ATAPI-6 T13/1410D revision 2",
	0x001c: "ATA/ATAPI-6 T13/1410D revision 1",
	0x001d: "ATA/ATAPI-7 published, ANSI INCITS 397-2005",
	0x001e: "ATA/ATAPI-7 T13/1532D revision 0",
	0x001f: "ACS-3 T13/2161-D revision 3b",
	0x0021: "ATA/ATAPI-7 T13/1532D revision 4a",
	0x0022: "ATA/ATAPI-6 published, ANSI INCITS 361-2002",
	0x0027: "ATA8-ACS T13/1699-D revision 3c",
	0x0028: "ATA8-ACS T13/1699-D revision 6",
	0x0029: "ATA8-ACS T13/1699-D revision 4",
	0x0031: "ACS-2 T13/2015-D revision 2",
	0x0033: "ATA8-ACS T13/1699-D revision 3e",
	0x0042: "ATA8-ACS T13/1699-D revision 3f",
	0x0052: "ATA8-ACS T13/1699-D revision 3b",
	0x005e: "ACS-4 T13/BSR INCITS 529 revision 5",
	0x006d: "ACS-3 T13/2161-D revision 5",
	0x0082: "ACS-2 published, ANSI INCITS 482-2012",
	0x0107: "ATA8-ACS T13/1699-D revision 2d",
	0x010a: "ACS-3 published, ANSI INCITS 522-2014",
	0x0110: "ACS-2 T13/2015-D revision 3",
	0x011b: "ACS-3 T13/2161-D revision 4",
	0x0039: "ATA8-ACS T13/1699-D revision 4c",
}
