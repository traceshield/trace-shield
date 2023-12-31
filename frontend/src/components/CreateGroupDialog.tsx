import { useState } from 'react';
import { useQuery, useMutation } from '@apollo/client';
import { Theme, useTheme } from '@mui/material/styles';
import {
  Box,
  Button,
  TextField,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  OutlinedInput,
  InputLabel,
  MenuItem,
  FormControl,
  Select
} from '@mui/material';
import { SelectChangeEvent } from '@mui/material/Select';
import { useUpdateGroupMutation, namedOperations, useListUsersQuery, UserFragment, UserInput } from '../generated/graphql';

const ITEM_HEIGHT = 48;
const ITEM_PADDING_TOP = 8;
const MenuProps = {
  PaperProps: {
    style: {
      maxHeight: ITEM_HEIGHT * 4.5 + ITEM_PADDING_TOP,
      width: 250
    }
  }
};

function getStyles(name: string, names: UserInput[], theme: Theme) {
  const ids = names.map((n) => n.id);
  return {
    fontWeight:
    ids.indexOf(name) === -1
        ? theme.typography.fontWeightRegular
        : theme.typography.fontWeightMedium
  };
}

function CreateGroupDialog() {
  const theme = useTheme();
  const [open, setOpen] = useState(false);
  const [groupName, setGroupName] = useState('');
  const [users, setUsers] = useState<UserInput[]>([]);

  const {
    data: usersData,
    loading: usersLoading,
    error: usersError
  } = useListUsersQuery();
  const [
    createGroup,
    { data: groupsData, loading: groupsLoading, error: groupsError }
  ] = useUpdateGroupMutation({
    variables: {
      name: groupName,
      members: users
    },
    refetchQueries: [namedOperations.Query.ListGroups]
  });

  const handleClickOpen = () => {
    setOpen(true);
  };

  const handleClose = () => {
    setGroupName('');
    setUsers([]);
    setOpen(false);
  };

  const handleSave = () => {
    createGroup();
    handleClose();
  };

  const isSaveButtonDisabled = () => {
    if (!groupName || !users) {
      return true;
    }

    return false;
  };

  const handleChange = (event: SelectChangeEvent<typeof users>) => {
    const {
      target: { value }
    } = event;
    const input: UserInput[] = [];
    if (value) {
      if (typeof value === 'string') {
        const ids = value.split(',')
        for (let i = 0; i < ids.length; i++) {
          input.push({ id: ids[i] });
        }
      } else {
        for (let i = 0; i < value.length; i++) {
          input.push({ id: value[i].id });
        }
      }
    }
    setUsers(
      // On autofill we get a stringified value.
      input
    );
  };

  return (
    <div>
      <Box
        sx={{
          display: 'flex',
          width: '100%',
          flexDirection: 'row',
          justifyContent: 'end'
        }}
      >
        <Button variant="contained" onClick={handleClickOpen}>
          Create group
        </Button>
      </Box>
      <Dialog open={open} onClose={handleClose}>
        <DialogTitle>Create Group</DialogTitle>
        <DialogContent>
          <TextField
            sx={{ marginTop: 1 }}
            required
            fullWidth
            label="Group Name"
            value={groupName}
            onChange={e => setGroupName(e.target.value)}
          />
          <FormControl fullWidth sx={{ marginTop: 2, minWidth: 500 }}>
            <InputLabel id="demo-multiple-name-label">Select Users</InputLabel>
            <Select
              labelId="demo-multiple-name-label"
              id="demo-multiple-name"
              multiple
              value={users}
              onChange={handleChange}
              input={<OutlinedInput label="Select Users" />}
              MenuProps={MenuProps}
            >
              {usersData
                ? usersData.listUsers.map((user: UserFragment) => (
                    <MenuItem
                      key={user.email}
                      value={user.id}
                      style={getStyles(user.email, users, theme)}
                    >
                      {user.email}
                    </MenuItem>
                  ))
                : null}
            </Select>
          </FormControl>
        </DialogContent>
        <DialogActions>
          <Button variant="contained" color="info" onClick={handleClose}>
            Cancel
          </Button>
          {isSaveButtonDisabled() ? (
            <Button variant="contained" disabled>
              Save
            </Button>
          ) : (
            <Button variant="contained" onClick={handleSave}>
              Save
            </Button>
          )}
        </DialogActions>
      </Dialog>
    </div>
  );
}

export default CreateGroupDialog;
